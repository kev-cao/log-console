package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/structs"
	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var teardownK3sCmd = &cobra.Command{
	Use:   "k3s",
	Short: "Disables and uninstalls K3S services.",
	Run: func(cmd *cobra.Command, _ []string) {
		dispatcher, err := dispatchers.GetDispatcher(
			structs.Map(globalTearDownFlags),
			dispatchMethod(globalTearDownFlags.Method),
		)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer dispatcher.Cleanup()
		fmt.Println(header("Tearing down K3S..."))
		if err := teardownK3s(dispatcher); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("Tear down successful.")
		return
	},
	TraverseChildren: true,
}

func init() {
	teardownCmd.AddCommand(teardownK3sCmd)
}

func teardownK3s(d dispatch.ClusterDispatcher) error {
	master := d.GetMasterNode()
	if err := d.SendCommands(
		master,
		dispatch.NewCommand(
			"/usr/local/bin/k3s-uninstall.sh",
			dispatch.WithOsPipe(),
			dispatch.WithPrefixWriter(master),
		),
	); err != nil {
		return fmt.Errorf("error tearing down K3S: %w", err)
	}

	var wg errgroup.Group
	for _, node := range d.GetWorkerNodes() {
		wg.Go(func() error {
			return d.SendCommands(
				node,
				dispatch.NewCommand(
					"/usr/local/bin/k3s-agent-uninstall.sh",
					dispatch.WithOsPipe(),
					dispatch.WithPrefixWriter(node),
				),
			)
		})
	}
	if err := wg.Wait(); err != nil {
		return fmt.Errorf("error tearing down K3S: %w", err)
	}
	return nil
}
