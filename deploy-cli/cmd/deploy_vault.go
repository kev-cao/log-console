package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/utils/pathutils"
	"github.com/kev-cao/log-console/utils/sliceutils"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var deployVaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Deploys a vault server to the cluster.",
	Long: `It deploys a vault server to the cluster and initializes it with the provided
credentials.`,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := cmd.ValidateRequiredFlags(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		globalVaultFlags.deployFlags = globalDeployFlags
		if err := globalVaultFlags.validate(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		dispatcher, err := dispatchers.GetDispatcher(
			structs.Map(globalVaultFlags),
			dispatchMethod(globalVaultFlags.Method),
		)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := deployVault(dispatcher); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
	TraverseChildren: true,
}

var globalVaultFlags vaultFlags

func init() {
	deployCmd.AddCommand(deployVaultCmd)
	deployVaultCmd.Flags().StringVar(
		&globalVaultFlags.Creds,
		"creds",
		"",
		"Path to the cloud credentials file",
	)

	deployVaultCmd.MarkFlagRequired("creds")
}

type vaultFlags struct {
	deployFlags
	Creds string
}

func (f *vaultFlags) validate() error {
	if f.Creds == "" {
		return errors.New("Cloud credentials file must be provided.")
	}
	creds, _ := pathutils.AbsolutePath(f.Creds)
	if _, err := os.Stat(creds); os.IsNotExist(err) {
		return errors.New("Cloud credentials file does not exist.")
	}
	return nil
}

var kubeEnv map[string]string = map[string]string{
	"KUBECONFIG": "/etc/rancher/k3s/k3s.yaml",
}

func deployVault(d dispatch.ClusterDispatcher) error {
	fmt.Println(header("Installing Helm..."))
	if err := installHelm(d); err != nil {
		return err
	}
	fmt.Println(header("Installing Cert-Manager..."))
	if err := installCertManager(d); err != nil {
		return err
	}
	fmt.Println(header("Creating Vault resources..."))
	if err := createVaultResources(d); err != nil {
		return err
	}
	fmt.Println(header("Setting up TLS certificates..."))
	if err := createCertificates(d); err != nil {
		return err
	}
	fmt.Println(header("Initializing Vault..."))
	if err := initVault(d); err != nil {
		return err
	}
	return nil
}

// installHelm installs helm on the master node in order to install the
// vault chart.
func installHelm(d dispatch.ClusterDispatcher) error {
	// Check if helm is installed
	installed, err := checkInstall(
		d, d.GetMasterNode(), dispatch.NewCommand("helm version"), nil,
	)
	if installed {
		fmt.Println("Helm already installed")
		return nil
	}
	if err != nil {
		return err
	}
	// Helm not installed (exit code 127), install it
	if err := d.SendCommands(
		d.GetMasterNode(),
		dispatch.NewCommands(
			[]string{
				"curl -fsSL -o /tmp/install-helm.sh " +
					"https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3",
				"chmod u+x /tmp/install-helm.sh",
				"/tmp/install-helm.sh",
			},
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(d.GetMasterNode()),
		)...,
	); err != nil {
		return fmt.Errorf("error installing helm: %w", err)
	}
	return nil
}

// installCertManager installs cert-manager on the cluster to manage the
// signing and auto-rotating of certificates for the vault server.
func installCertManager(d dispatch.ClusterDispatcher) error {
	master := d.GetMasterNode()
	var err error
	if err := d.SendCommands(
		master,
		dispatch.NewCommands(
			[]string{
				"helm repo add jetstack https://charts.jetstack.io",
				"helm install " +
					"cert-manager jetstack/cert-manager " +
					"--namespace cert-manager " +
					"--create-namespace " +
					"--version v1.16.1 " +
					"--set disableAutoApproval=true " +
					"--set crds.enabled=true",
			},
			dispatch.WithEnv(kubeEnv),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(master),
		)...,
	); err != nil {
		return fmt.Errorf("error installing cert-manager: %w", err)
	}

	if err := d.SendCommands(
		master,
		dispatch.NewCommands(
			[]string{
				// Install cert-manager approver policy. Must be installed before
				// trust manager.
				"helm upgrade cert-manager-approver-policy " +
					"jetstack/cert-manager-approver-policy " +
					"--install --namespace cert-manager --wait",
				// Install cert-manager trust manager
				"helm upgrade trust-manager jetstack/trust-manager " +
					"--install --namespace cert-manager --wait " +
					// Flags required when pairing with cert-manager-approver-policy
					"--set app.webhook.tls.approverPolicy.enabled=true " +
					"--set app.webhook.tls.approverPolicy.certManagerNamespace=cert-manager",
			},
			dispatch.WithEnv(kubeEnv),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(master),
		)...,
	); err != nil {
		return fmt.Errorf("error installing cert-manager extensions: %w", err)
	}

	var goInstalled bool
	if goInstalled, err = checkInstall(
		d, d.GetMasterNode(), dispatch.NewCommand("go version"), nil,
	); err != nil {
		return fmt.Errorf("error checking go installation: %w", err)
	}
	if !goInstalled {
		if err := d.SendCommands(
			d.GetMasterNode(),
			dispatch.NewCommand(
				"sudo apt install golang-go --yes",
				dispatch.WithOsPipe(),
				dispatch.WithPrefixWriter(d.GetMasterNode()),
			),
		); err != nil {
			return fmt.Errorf("error installing go: %w", err)
		}
	}
	if goInstalled, err = checkInstall(
		d, d.GetMasterNode(), dispatch.NewCommand("cmctl help"), nil,
	); err != nil {
		return fmt.Errorf("error checking cmctl installation: %w", err)
	}
	if !goInstalled {
		if err := d.SendCommands(
			d.GetMasterNode(),
			dispatch.NewCommand(
				"go install github.com/cert-manager/cmctl/v2@latest",
				dispatch.WithOsPipe(),
				dispatch.WithPrefixWriter(d.GetMasterNode()),
			),
		); err != nil {
			return fmt.Errorf("error installing cmctl: %w", err)
		}
	}

	if err := d.SendCommands(
		d.GetMasterNode(),
		dispatch.NewCommand(
			"$(go env GOPATH)/bin/cmctl check api --wait=2m",
			dispatch.WithEnv(kubeEnv),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(d.GetMasterNode()),
		),
	); err != nil {
		return fmt.Errorf("error performing wait check for cert-manager: %w", err)
	}
	return nil
}

// createVaultResources creates the K8s resources required by the vault server.
func createVaultResources(d dispatch.ClusterDispatcher) error {
	var wg errgroup.Group
	for _, node := range d.GetNodes() {
		func(node dispatch.Node) {
			wg.Go(func() error {
				if ret := d.SendCommands(
					node,
					dispatch.NewCommand(
						"sudo mkdir -p /srv/cluster/storage/vault",
						dispatch.WithOsPipe(),
						dispatch.WithPrefixWriter(node),
					),
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

	master := d.GetMasterNode()
	creds, _ := pathutils.AbsolutePath(globalVaultFlags.Creds)
	if err := d.SendFile(
		master,
		creds,
		"/home/ubuntu/etc/log-console/credentials.json",
	); err != nil {
		return fmt.Errorf("error sending credentials file to master node: %w", err)
	}

	if err := d.SendCommands(
		master,
		dispatch.NewCommands(
			[]string{
				"kubectl apply -f ~/log-console/k8s/vault.yaml",
				"kubectl create secret generic kms -n vault " +
					"--from-file=/home/ubuntu/etc/log-console/credentials.json",
			},
			dispatch.WithEnv(kubeEnv),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(master),
		)...,
	); err != nil {
		return fmt.Errorf("error creating vault resources: %w", err)
	}
	return nil
}

// createCertificates creates the certificates required by the vault server
// and sets up the trust bundles.
func createCertificates(d dispatch.ClusterDispatcher) error {
	// Create certificates
	master := d.GetMasterNode()
	if err := d.SendCommands(
		master,
		dispatch.NewCommand(
			"kubectl apply -f ~/log-console/k8s/certificates.yaml",
			dispatch.WithEnv(kubeEnv),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(master),
		),
	); err != nil {
		return fmt.Errorf("error creating certificates: %w", err)
	}
	return nil
}

// initVault initializes the vault server using the helm chart.
func initVault(d dispatch.ClusterDispatcher) error {
	if err := d.SendCommands(
		d.GetMasterNode(),
		dispatch.NewCommands(
			[]string{
				"helm repo add hashicorp https://helm.releases.hashicorp.com",
				"helm repo update",
				"helm install vault hashicorp/vault " +
					"-f ~/log-console/k8s/vault-overrides.yaml " +
					"--namespace vault",
			},
			dispatch.WithEnv(kubeEnv),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(d.GetMasterNode()),
		)...,
	); err != nil {
		return fmt.Errorf("error initializing vault: %w", err)
	}

	fmt.Println("Waiting for vault pod to be running...")
	if err := waitVaultPods(d); err != nil {
		return err
	}
	fmt.Println("Vault pods running. Unsealing vault...")

	if err := d.SendCommands(
		d.GetMasterNode(),
		dispatch.NewCommand(
			`kubectl exec -n vault vault-0 -- /bin/ash -c `+
				`"export VAULT_SKIP_VERIFY=1; vault operator init"`,
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(d.GetMasterNode()),
		),
	); err != nil {
		return fmt.Errorf("error initializing vault: %w", err)
	}

	return nil
}

func waitVaultPods(d dispatch.ClusterDispatcher) error {
	// Get all vault pod names
	var output bytes.Buffer
	if err := d.SendCommands(
		d.GetMasterNode(),
		dispatch.NewCommand(
			`kubectl get pods -n vault --template `+
				`'{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}' | grep "^vault-[0-9]\+"`,
			dispatch.WithStdout(&output),
			dispatch.WithStderr(dispatch.NewPrefixWriter(
				d.GetMasterNode().Name,
				os.Stderr,
			)),
		),
	); err != nil {
		return fmt.Errorf("error getting vault pod names: %w", err)
	}
	pods := strings.Split(strings.TrimSpace(output.String()), "\n")

	if err := d.SendCommands(
		d.GetMasterNode(),
		dispatch.NewCommand(
			fmt.Sprintf(
				"kubectl wait --for=condition=Ready --timeout=300s -n vault %s",
				strings.Join(
					sliceutils.Map(pods, func(name string, _ int) string {
						return "pod/" + name
					}),
					" ",
				),
			),
			dispatch.WithTimeout(330*time.Second),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(d.GetMasterNode()),
		),
	); err != nil {
		return fmt.Errorf("error waiting for vault pods to be ready: %w", err)
	}
	return nil
}
