package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/fatih/structs"
	"github.com/kev-cao/log-console/deploy-cli/dispatch"
	"github.com/spf13/cobra"
)

var teardownCmd = &cobra.Command{
	Use:   "teardown",
	Short: "Provides functionality for tearing down a cluster.",
	Long: `Provides functionality for tearing down a cluster.
	It resets the cluster to its initial state before deployment.`,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		if err := cmd.ValidateRequiredFlags(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := globalTearDownFlags.validate(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
	Run: func(_ *cobra.Command, _ []string) {
		dispatcher, err := dispatchers.GetDispatcher(
			structs.Map(globalTearDownFlags),
			dispatchMethod(globalTearDownFlags.Method),
		)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer dispatcher.Cleanup()
		fmt.Println(header("Tearing down everything..."))
		customTeardown, ok := dispatcher.(interface{ Teardown() error })
		if ok {
			if err := customTeardown.Teardown(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		} else {
			if err := teardownAll(dispatcher); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		fmt.Println("Tear down successful.")
		return
	},
}

var globalTearDownFlags teardownFlags

func init() {
	rootCmd.AddCommand(teardownCmd)
	teardownCmd.PersistentFlags().StringVar(
		&globalTearDownFlags.Method,
		"method",
		"",
		"Deployment method to teardown (multipass, ssh, local)",
	)
	teardownCmd.PersistentFlags().IntVarP(
		&globalTearDownFlags.NumNodes,
		"nodes",
		"n",
		3,
		"Number of nodes to teardown",
	)

	teardownCmd.MarkFlagRequired("method")
	teardownCmd.MarkFlagRequired("nodes")
}

type teardownFlags struct {
	Method   string
	NumNodes int
}

func (f *teardownFlags) validate() error {
	validMethods := []string{"multipass", "ssh", "local"}
	if !slices.Contains(validMethods, f.Method) {
		return errors.New(
			fmt.Sprintf(
				"Unsupported deployment method. Must be one of %v",
				validMethods,
			),
		)
	}

	if f.NumNodes <= 0 {
		return errors.New("Number of nodes must be greater than 0.")
	}
	return nil
}

func teardownAll(d dispatch.ClusterDispatcher) error {
	if err := teardownVault(d); err != nil {
		return err
	}
	return nil
}
