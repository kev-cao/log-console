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
	multipassCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := multipassDispatcher.Init(); err != nil {
			fmt.Printf("Error initializing multipass cluster: %v\n", err)
			os.Exit(1)
		}
	},
}
