package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/kev-cao/log-console/deploy-cli/dispatch/multipass"
	"github.com/kev-cao/log-console/utils/waitutils"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploys a cluster and initializes it",
	Long: `The deploy command is used to deploy a cluster and initialize it for use. It downloads the project 
onto the cluster and optionally sets up K3S on the cluster.`,
	PersistentPreRun: runDeploy,
	Run: func(cmd *cobra.Command, args []string) {
		// Do nothing as deploy is handled in PersistentPreRun
	},
}

func runDeploy(cmd *cobra.Command, _ []string) {
	if err := cmd.ValidateRequiredFlags(); err != nil {
		cobra.CheckErr(err)
	}
	if err := globalDeployFlags.validate(); err != nil {
		cobra.CheckErr(err)
	}
	dispatcher, err := dispatchers.GetDispatcher(
		structs.Map(globalDeployFlags),
		dispatchMethod(globalDeployFlags.Method),
	)
	if err != nil {
		cobra.CheckErr(err)
	}
	if globalDeployFlags.Launch {
		mpDispatcher := dispatcher.(*multipass.MultipassDispatcher)

		fmt.Println(header("Launching nodes..."))
		if err := mpDispatcher.LaunchNodes(); err != nil {
			cobra.CheckErr(err)
		}
	}
	fmt.Println(header("Waiting for cluster to be ready..."))
	if err := waitReady(dispatcher); err != nil {
		cobra.CheckErr(err)
	}
	fmt.Println("Cluster ready.")
	if globalDeployFlags.DownloadProject {
		fmt.Println(header("Downloading project..."))
		if err := downloadProject(dispatcher); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Println("Project downloaded.")
	}
	if globalDeployFlags.SetupK3S {
		fmt.Println(header("Setting up K3S on the cluster..."))
		if err := setupK3S(dispatcher); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Println("K3S setup complete.")
	}
}

var globalDeployFlags = deployFlags{
	Env: DEV,
}

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.PersistentFlags().VarP(
		&globalDeployFlags.Method,
		"method",
		"m",
		fmt.Sprintf("Method to use for deployment. Options: %v", dispatchMethodOptions),
	)
	deployCmd.PersistentFlags().VarP(
		&globalDeployFlags.Env,
		"env",
		"e",
		fmt.Sprintf("Deployment environment. Options: %v", envOptions),
	)
	deployCmd.PersistentFlags().IntVarP(
		&globalDeployFlags.NumNodes,
		"nodes",
		"n",
		3,
		"Number of nodes to deploy",
	)
	deployCmd.PersistentFlags().BoolVar(
		&globalDeployFlags.Launch,
		"launch",
		false,
		"Whether to launch the nodes before deployment (only for multipass)",
	)
	deployCmd.PersistentFlags().StringSliceVarP(
		&globalDeployFlags.Remotes,
		"remotes",
		"r",
		nil,
		"User-qualified hostnames for each remote node (required for SSH deployments). First address is the master node.",
	)
	deployCmd.PersistentFlags().StringVarP(
		&globalDeployFlags.IdentityFile,
		"identity_file",
		"i",
		"~/.ssh/id_rsa",
		"The identity (private key) file to use for SSH deployments.",
	)
	deployCmd.PersistentFlags().BoolVar(
		&globalDeployFlags.SetupK3S,
		"k3s",
		false,
		"Whether to setup K3S on the cluster (defaults true for multipass)",
	)
	deployCmd.PersistentFlags().BoolVar(
		&globalDeployFlags.DownloadProject,
		"download",
		false,
		"Whether to download the project onto the cluster (defaults true for multipass)",
	)

	deployCmd.MarkPersistentFlagRequired("nodes")
	deployCmd.MarkPersistentFlagRequired("method")
}

func waitReady(d dispatch.ClusterDispatcher) error {
	if err := waitutils.WaitFunc(d.Ready, 5*time.Second, 1*time.Second); err != nil {
		return errors.New("Cluster not ready for deployment. " +
			"Make sure the cluster is initialized first.")
	}
	return nil
}

func downloadProject(d dispatch.ClusterDispatcher) error {
	var source string
	switch globalDeployFlags.Env {
	case DEV:
		path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return fmt.Errorf("error getting project path: %w", err)
		}
		source = "local://" + strings.TrimSpace(string(path))
	case PROD:
		source = "git@github.com:kev-cao/log-console.git"
	default:
		return errors.New("invalid environment")
	}
	if err := d.DownloadProject(d.GetMasterNode(), source); err != nil {
		return fmt.Errorf("error downloading project: %w", err)
	}
	return nil
}

func setupK3S(d dispatch.ClusterDispatcher) error {
	if err := maybeTeardownK3S(d); err != nil {
		return err
	}

	// Start K3S daemon on master node
	masterNode := d.GetMasterNode()
	if err := d.SendCommands(
		masterNode,
		dispatch.NewCommand(
			fmt.Sprintf(
				`curl -sfL https://get.k3s.io | K3S_NODE_NAME=%s K3S_KUBECONFIG_MODE=644 sh -`,
				masterNode.Kubename,
			),
			dispatch.WithTimeout(3*time.Minute),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(masterNode),
		),
	); err != nil {
		return err
	}

	time.Sleep(3 * time.Second) // Give K3S time to start

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Get connection params for worker nodes
	url := fmt.Sprintf("https://%s:6443", masterNode.Remote.FQDN)
	token, err := getK3SNodeToken(d, masterNode)
	if err != nil {
		return err
	}
	var wg errgroup.Group
	for _, node := range d.GetWorkerNodes() {
		wg.Go(func() error {
			if err := d.SendCommandsContext(
				ctx,
				node,
				dispatch.NewCommand(
					fmt.Sprintf(
						`curl -sfL https://get.k3s.io | K3S_NODE_NAME="%s" K3S_URL="%s" K3S_TOKEN="%s" sh -`,
						node.Kubename, url, token,
					),
					dispatch.WithTimeout(3*time.Minute),
					dispatch.WithOsPipe(),
					dispatch.WithPrefixWriter(node),
				),
			); err != nil {
				return err
			}
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return err
	}
	return nil
}

func getK3SNodeToken(d dispatch.ClusterDispatcher, node dispatch.Node) (string, error) {
	var token strings.Builder
	cmd := dispatch.NewCommand(
		"sudo cat /var/lib/rancher/k3s/server/node-token",
		dispatch.WithTimeout(10*time.Second),
		dispatch.WithOsPipe(),
		dispatch.WithPrefixWriter(node),
		dispatch.WithStdout(&token),
	)
	if err := d.SendCommands(
		node,
		cmd,
	); err != nil {
		return "", err
	}
	return strings.TrimSpace(token.String()), nil
}

func maybeTeardownK3S(d dispatch.ClusterDispatcher) error {
	var wg errgroup.Group
	nodes := append([]dispatch.Node{d.GetMasterNode()}, d.GetWorkerNodes()...)
	for idx, node := range nodes {
		wg.Go(func() error {
			var status strings.Builder
			if err := d.SendCommands(
				node,
				dispatch.NewCommand(
					// Adding sleep as workaround for multipass issue where command gets stuck in loop
					// https://github.com/canonical/multipass/issues/3771
					"systemctl is-active k3s & sleep 1",
					dispatch.WithStdout(&status),
					dispatch.WithTimeout(10*time.Second),
				),
			); err != nil {
				return err
			}
			if strings.TrimSpace(status.String()) == "active" {
				fmt.Printf("Uninstalling K3S on %s...\n", node.Name)
				var uninstallCmd string
				if idx == 0 {
					uninstallCmd = "/usr/local/bin/k3s-uninstall.sh"
				} else {
					uninstallCmd = fmt.Sprintf("/usr/local/bin/k3s-agent-uninstall.sh")
				}
				return d.SendCommands(
					node,
					dispatch.NewCommand(
						uninstallCmd,
						dispatch.WithTimeout(2*time.Minute),
						dispatch.WithOsPipe(),
						dispatch.WithPrefixWriter(node),
					),
				)
			}
			return nil
		})
	}
	return wg.Wait()
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
	err := d.SendCommands(node, cmd)
	if err == nil {
		if postFunc == nil {
			return true, nil
		}
		return postFunc(), nil
	}

	var code int
	// Not receiving an exit error means the command failed to run for an unexpected reason
	switch t := err.(type) {
	case *exec.ExitError:
		code = t.ExitCode()
	case *ssh.ExitError:
		code = t.ExitStatus()
	default:
		return false, err
	}
	// If the exit code is not 127, the command failed not because of a missing command
	if code != 127 {
		return false, err
	}
	return false, nil
}
