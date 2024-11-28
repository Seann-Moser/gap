/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/Seann-Moser/gpa/tools"
	"github.com/spf13/cobra"
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
	rootCmd.AddCommand(analyzeCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// analyizeCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// analyizeCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func Analyze(cmd *cobra.Command, args []string) error {
	data, err := tools.ListFunctions("")
	if err != nil {
		return err
	}
	tools.PrintFunctionInfos(data)

	//f, err := tools.GetFunctionWithComments(data[rand.Intn(len(data))])
	//if err != nil {
	//	return err
	//}
	//println(f)
	return tools.GenerateGraphviz(data, "graph.dot")
}
