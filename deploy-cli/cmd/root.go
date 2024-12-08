/*
Copyright © 2024 Kevin Cao <kcao1998@gmail.com>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "deploy-cli",
	Short: "CLI tool for deploying log-console to a Kubernetes cluster.",
	Long: `CLI tool for deploying log-console to a Kubernetes cluster.
	It provides a simple way to deploy the app to a variety of environments.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		cobra.CheckErr(err)
	}
}
