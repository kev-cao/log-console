package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
)

var teardownCmd = &cobra.Command{
	Use:   "teardown",
	Short: "Provides functionality for tearing down a cluster.",
	Long: `Provides functionality for tearing down a cluster.
	It resets the cluster to its initial state before deployment.`,
	Run: func(_ *cobra.Command, _ []string) {
		if err := tdFlags.validate(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		dispatcher, err := getDispatcher(tdFlags.method)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := dispatcher.TearDown(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	},
}

func init() {
	rootCmd.AddCommand(teardownCmd)
	teardownCmd.Flags().StringVar(&tdFlags.method, "method", "", "Deployment method to teardown (multipass, ssh, local)")
}

type teardownFlags struct {
	method string
}

var tdFlags = teardownFlags{}

func (f *teardownFlags) validate() error {
	validMethods := []string{"multipass", "ssh", "local"}
	if !slices.Contains(validMethods, f.method) {
		return errors.New(
			fmt.Sprintf(
				"Unsupported deployment method. Must be one of %v",
				validMethods,
			),
		)
	}
	return nil
}
