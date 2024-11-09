/*
Copyright © 2024 Kevin Cao <kcao1998@gmail.com>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var multipassCmd = &cobra.Command{
	Use:   "multipass",
	Short: "Provides functionality for starting/stopping a multipass cluster.",
	Long: `Provides functionality for starting/stopping a multipass cluster. 
		It manages a multipass cluster that is configured with Kubernetes and is set
		up specifically for the log-console application.`,
}

func init() {
	rootCmd.AddCommand(multipassCmd)

	// Multipass subcommands
	multipassCmd.AddCommand(launchCmd)
}

var launchCmd = &cobra.Command{
	Use:   "launch",
	Short: "Launches the multipass nodes.",
	Long:  `Launches the multipass nodes.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := multipassDispatcher.LaunchNodes(); err != nil {
			fmt.Printf("Error initializing multipass cluster: %v\n", err)
			os.Exit(1)
		}
	},
}
