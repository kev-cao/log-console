package cmd

import (
	"errors"
	"fmt"

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
			cobra.CheckErr(err)
		}
		if err := globalTearDownFlags.validate(); err != nil {
			cobra.CheckErr(err)
		}
	},
	Run: func(_ *cobra.Command, _ []string) {
		dispatcher, err := dispatchers.GetDispatcher(
			structs.Map(globalTearDownFlags),
			dispatchMethod(globalTearDownFlags.Method),
		)
		if err != nil {
			cobra.CheckErr(err)
		}
		defer dispatcher.Cleanup()
		fmt.Println(header("Tearing down everything..."))
		customTeardown, ok := dispatcher.(interface{ Teardown() error })
		if ok {
			if err := customTeardown.Teardown(); err != nil {
				cobra.CheckErr(err)
			}
		} else {
			if err := teardownAll(dispatcher); err != nil {
				cobra.CheckErr(err)
			}
		}
		fmt.Println("Tear down successful.")
		return
	},
}

var globalTearDownFlags teardownFlags

func init() {
	rootCmd.AddCommand(teardownCmd)
	teardownCmd.PersistentFlags().VarP(
		&globalTearDownFlags.Method,
		"method",
		"m",
		fmt.Sprintf("Deployment method. Options: %v", dispatchMethodOptions),
	)
	teardownCmd.PersistentFlags().IntVarP(
		&globalTearDownFlags.NumNodes,
		"nodes",
		"n",
		3,
		"Number of nodes to teardown",
	)
	teardownCmd.PersistentFlags().StringSliceVarP(
		&globalTearDownFlags.Remotes,
		"remotes",
		"r",
		nil,
		"User-qualified hostnames for each remote node (required for SSH deployments). First address is the master node.",
	)
	teardownCmd.PersistentFlags().StringVarP(
		&globalTearDownFlags.IdentityFile,
		"identity_file",
		"i",
		"~/.ssh/id_rsa",
		"The identity (private key) file to use for SSH deployments.",
	)

	teardownCmd.MarkPersistentFlagRequired("method")
	teardownCmd.MarkPersistentFlagRequired("nodes")
}

type teardownFlags struct {
	Method       dispatchMethod
	NumNodes     int
	Remotes      []string
	IdentityFile string
}

func (f *teardownFlags) validate() error {
	if f.NumNodes <= 0 {
		return errors.New("Number of nodes must be greater than 0.")
	}

	if f.Method == SSH {
		if len(f.Remotes) == 0 {
			return errors.New("Remote addresses must be provided for SSH deployments.")
		} else if len(f.Remotes) != f.NumNodes {
			return errors.New("Number of remotes must match number of nodes.")
		} else if f.IdentityFile == "" {
			return errors.New("Private key file must be provided for SSH deployments.")
		}
	}
	return nil
}

func teardownAll(d dispatch.ClusterDispatcher) error {
	if err := teardownVault(d); err != nil {
		return err
	}
	return nil
}
