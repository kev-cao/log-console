package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/deploy-cli/dispatch/multipass"
	"github.com/kev-cao/log-console/utils/path"
	"github.com/kev-cao/log-console/utils/slice"
	"github.com/kev-cao/log-console/utils/wait"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploys the log-console application to a Kubernetes cluster.",
	Long: `Deploys the log-console application to a Kubernetes cluster.
	
	Specify the deployment method with --method.
	`,
	Run: func(_ *cobra.Command, _ []string) {
		if err := flags.validate(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		dispatcher, err := getDispatcher(flags.method)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if flags.init {
			mpDispatcher := dispatcher.(multipass.MultipassDispatcher)
			if err := mpDispatcher.Init(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}

		if err := deploy(dispatcher); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringVar(&flags.method, "method", "", "Method to use for deployment (multipass, ssh, local)")
	deployCmd.Flags().StringVar(&flags.creds, "creds", "", "Path to cloud credentials file for deployment")
	deployCmd.Flags().StringVar(&flags.env, "env", "dev", "Which environment deployment is for (dev, prod)")
	deployCmd.Flags().BoolVar(&flags.init, "init", false, "Whether to run the init command before deploying (only for multipass)")
}

type deployFlags struct {
	method string
	creds  string
	env    string
	init   bool
}

var flags = deployFlags{}

func (f *deployFlags) validate() error {
	validMethods := []string{"multipass", "ssh", "local"}
	if !slices.Contains(validMethods, f.method) {
		return errors.New(
			fmt.Sprintf(
				"Unsupported deployment method. Must be one of %v",
				validMethods,
			),
		)
	}

	if f.init && f.method != "multipass" {
		return errors.New("Init flag is only supported for multipass deployments.")
	}

	if f.creds == "" {
		return errors.New("Cloud credentials file must be provided.")
	}
	creds, _ := path.AbsolutePath(f.creds)
	if _, err := os.Stat(creds); os.IsNotExist(err) {
		return errors.New("Cloud credentials file does not exist.")
	}

	if f.env != "dev" && f.env != "prod" {
		return errors.New("Environment must be either dev or prod.")
	}

	return nil
}

func deploy(d dispatch.ClusterDispatcher) error {
	fmt.Println("Waiting for cluster to be ready...")
	if err := waitReady(d); err != nil {
		return err
	}
	fmt.Println("Cluster ready. Beginning deployment...")
	fmt.Println("Downloading project...")
	if err := downloadProject(d); err != nil {
		return err
	}
	fmt.Println("Installing Helm...")
	if err := installHelm(d); err != nil {
		return err
	}
	fmt.Println("Installing Cert-Manager...")
	if err := installCertManager(d); err != nil {
		return err
	}
	fmt.Println("Initializing Vault...")
	if err := initVault(d); err != nil {
		return err
	}
	return nil
}

func waitReady(d dispatch.ClusterDispatcher) error {
	if err := wait.WaitFunc(d.Ready, 5*time.Second, 1*time.Second); err != nil {
		return errors.New("Cluster not ready for deployment. " +
			"Make sure the cluster is initialized first.")
	}
	return nil
}

func downloadProject(d dispatch.ClusterDispatcher) error {
	var source string
	switch flags.env {
	case "dev":
		path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return fmt.Errorf("error getting project path: %w", err)
		}
		source = "local://" + strings.TrimSpace(string(path))
	case "prod":
		source = "git@github.com:kev-cao/log-console.git"
	}
	if err := d.DownloadProject(d.GetMasterNode(), source); err != nil {
		return fmt.Errorf("error downloading project: %w", err)
	}
	return nil
}

func installHelm(d dispatch.ClusterDispatcher) error {
	// Check if helm is installed
	installed, err := checkInstall(
		d, d.GetMasterNode(), dispatch.Command{Cmd: "helm version"}, nil,
	)
	if installed {
		fmt.Println("Helm already installed")
		return nil // Helm is already installed
	}
	if err != nil {
		return err
	}
	// Helm not installed (exit code 127), install it
	if err := d.SendCommand(
		d.GetMasterNode(),
		dispatch.NewCommands(
			d.GetMasterNode(),
			0,
			"curl -fsSL -o /tmp/install-helm.sh "+
				"https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3",
			"chmod u+x /tmp/install-helm.sh",
			"/tmp/install-helm.sh",
		)...,
	); err != nil {
		return fmt.Errorf("error installing helm: %w", err)
	}
	return nil
}

func installCertManager(d dispatch.ClusterDispatcher) error {
	var installed bool
	var err error
	if err := d.SendCommand(
		d.GetMasterNode(),
		dispatch.NewCommand(
			d.GetMasterNode(),
			"kubectl apply -f "+
				"https://github.com/cert-manager/cert-manager/releases/download/v1.15.3/cert-manager.yaml",
			nil,
			0,
		),
	); err != nil {
		return fmt.Errorf("error installing cert-manager: %w", err)
	}
	if installed, err = checkInstall(
		d, d.GetMasterNode(), dispatch.Command{Cmd: "go version"}, nil,
	); err != nil {
		return fmt.Errorf("error checking go installation: %w", err)
	}
	if !installed {
		if err := d.SendCommand(
			d.GetMasterNode(),
			dispatch.NewCommand(d.GetMasterNode(), "sudo apt install golang-go --yes", nil, 0),
		); err != nil {
			return fmt.Errorf("error installing go: %w", err)
		}
	}
	if installed, err = checkInstall(
		d, d.GetMasterNode(), dispatch.Command{Cmd: "cmctl help"}, nil,
	); err != nil {
		return fmt.Errorf("error checking cmctl installation: %w", err)
	}
	if !installed {
		if err := d.SendCommand(
			d.GetMasterNode(),
			dispatch.NewCommand(
				d.GetMasterNode(),
				"go install github.com/cert-manager/cmctl/v2@latest",
				nil,
				0,
			),
		); err != nil {
			return fmt.Errorf("error installing cmctl: %w", err)
		}
	}

	if err := d.SendCommand(
		d.GetMasterNode(),
		dispatch.NewCommand(
			d.GetMasterNode(),
			"$(go env GOPATH)/bin/cmctl check api --wait=2m",
			map[string]string{
				"KUBECONFIG": "/etc/rancher/k3s/k3s.yaml",
			},
			0,
		),
	); err != nil {
		return fmt.Errorf("error performing wait check for cert-manager: %w", err)
	}
	return nil
}

func initVault(d dispatch.ClusterDispatcher) error {
	var wg errgroup.Group
	for _, node := range d.GetNodes() {
		func(node dispatch.Node) {
			wg.Go(func() error {
				if ret := d.SendCommand(
					node,
					dispatch.NewCommand(node, "sudo mkdir -p /srv/cluster/storage/vault", nil, 0),
				); ret != nil {
					return ret
				}
				return nil
			})
		}(node)
	}
	if err := wg.Wait(); err != nil {
		return fmt.Errorf("error setting up vault directories: %w", err)
	}

	creds, _ := path.AbsolutePath(flags.creds)
	if err := d.SendFile(
		d.GetMasterNode(),
		creds,
		"/home/ubuntu/etc/log-console/credentials.json",
	); err != nil {
		return fmt.Errorf("error sending credentials file to master node: %w", err)
	}

	if err := d.SendCommandEnv(
		d.GetMasterNode(),
		map[string]string{
			"KUBECONFIG": "/etc/rancher/k3s/k3s.yaml",
		},
		dispatch.NewCommands(
			d.GetMasterNode(),
			0,
			"helm repo add hashicorp https://helm.releases.hashicorp.com",
			"helm repo update",
			"kubectl apply -f ~/log-console/k8s/vault.yaml",
			"kubectl apply -f ~/log-console/k8s/cert-manager.yaml",
			"kubectl create secret generic kms -n vault "+
				"--from-file=/home/ubuntu/etc/log-console/credentials.json",
			"helm install vault hashicorp/vault "+
				"-f ~/log-console/k8s/vault-overrides.yaml "+
				"--namespace vault",
		)...,
	); err != nil {
		return fmt.Errorf("error initializing vault: %w", err)
	}

	fmt.Println("Waiting for vault pod to be running...")
	if err := waitVaultPods(d); err != nil {
		return err
	}
	fmt.Println("Vault pods running. Unsealing vault...")

	if err := d.SendCommand(
		d.GetMasterNode(),
		dispatch.NewCommand(
			d.GetMasterNode(),
			`kubectl exec -n vault vault-0 -- /bin/ash -c `+
				`"export VAULT_SKIP_VERIFY=1; vault operator init"`,
			nil,
			0,
		),
	); err != nil {
		return fmt.Errorf("error initializing vault: %w", err)
	}

	return nil
}

func waitVaultPods(d dispatch.ClusterDispatcher) error {
	// Get all vault pod names
	var output bytes.Buffer
	if err := d.SendCommand(
		d.GetMasterNode(),
		dispatch.Command{
			Cmd: `kubectl get pods -n vault --template ` +
				`'{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}' | grep "^vault-[0-9]\+"`,
			Stdout: &output,
			Stderr: dispatch.NewPrefixWriter(d.GetMasterNode().Name, os.Stderr),
		},
	); err != nil {
		return fmt.Errorf("error getting vault pod names: %w", err)
	}
	pods := strings.Split(strings.TrimSpace(output.String()), "\n")

	if err := d.SendCommand(
		d.GetMasterNode(),
		dispatch.NewCommand(
			d.GetMasterNode(),
			fmt.Sprintf(
				"kubectl wait --for=condition=Ready --timeout=300s -n vault %s",
				strings.Join(
					slice.Map(pods, func(name string, _ int) string {
						return "pod/" + name
					}),
					" ",
				),
			),
			nil,
			330*time.Second,
		),
	); err != nil {
		return fmt.Errorf("error waiting for vault pods to be ready: %w", err)
	}
	return nil
}

// checkInstall checks if a command is installed on a node using the provided command.
// postFunc can be run if the command succeeds to do further checking (e.g. checking version number).
// Returns true if the command is installed, false if it is not, and an error if the command fails
// for an unexpected reason.
func checkInstall(
	d dispatch.ClusterDispatcher,
	node dispatch.Node,
	cmd dispatch.Command,
	postFunc func() bool,
) (bool, error) {
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := d.SendCommand(node, cmd)
	if err == nil {
		if postFunc == nil {
			return true, nil
		}
		return postFunc(), nil
	}

	exitErr, ok := err.(*exec.ExitError)
	// Not receiving an exit error means the command failed to run for an unexpected reason
	if !ok {
		return false, err
	}
	// If the exit code is not 127, the command failed not because of a missing command
	if exitErr.ExitCode() != 127 {
		return false, err
	}
	return false, nil
}
