package cmd

import (
	"fmt"

	"github.com/fatih/structs"
	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/spf13/cobra"
)

var teardownVaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Deletes Vault resources",
	Run: func(cmd *cobra.Command, _ []string) {
		dispatcher, err := dispatchers.GetDispatcher(
			structs.Map(globalTearDownFlags),
			dispatchMethod(globalTearDownFlags.Method),
		)
		if err != nil {
			cobra.CheckErr(err)
		}
		defer dispatcher.Cleanup()
		fmt.Println(header("Tearing down Vault..."))
		if err := teardownVault(dispatcher); err != nil {
			cobra.CheckErr(err)
		}
		fmt.Println("Tear down successful.")
		return
	},
	TraverseChildren: true,
}

func init() {
	teardownCmd.AddCommand(teardownVaultCmd)
}

func teardownVault(d dispatch.ClusterDispatcher) error {
	master := d.GetMasterNode()
	if err := d.SendCommands(
		master,
		dispatch.NewCommands(
			[]string{
				"helm uninstall vault -n vault --ignore-not-found",
				"helm uninstall cert-manager -n cert-manager --ignore-not-found",
				"helm uninstall cert-manager-approver-policy -n cert-manager --ignore-not-found",
				"helm uninstall trust-manager -n cert-manager --ignore-not-found",
				"kubectl delete -l app=vault --all-namespaces " +
					"$(kubectl api-resources --verbs=delete -o name | tr \"\\n\" \",\" | sed -e 's/,$//')",
			},
			dispatch.WithEnv(map[string]string{
				"KUBECONFIG": "/etc/rancher/k3s/k3s.yaml",
			}),
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(master),
		)...,
	); err != nil {
		return fmt.Errorf("error deleting vault resources: %w", err)
	}

	for _, node := range d.GetNodes() {
		if err := d.SendCommands(
			node,
			dispatch.NewCommand(
				"sudo rm -rf /srv/cluster/storage/vault",
				dispatch.WithOsPipe(),
				dispatch.WithPrefixWriter(node),
			),
		); err != nil {
			return fmt.Errorf("error cleaning up vault storage on %s: %w", node.Name, err)
		}
	}

	return nil
}
