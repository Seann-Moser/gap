/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/Seann-Moser/gpa/tools"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// analyizeCmd represents the analyize command
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: Analyze,
}

func init() {
	analyzeCmd.Flags().AddFlagSet(Flags())
	rootCmd.AddCommand(analyzeCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// analyizeCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// analyizeCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func Flags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("analyze", pflag.ExitOnError)
	fs.StringP("src", "s", "", "Path to the project")
	return fs
}

func Analyze(cmd *cobra.Command, args []string) error {

	return tools.Analyze(viper.GetString("src"), viper.GetString("output"))

}
