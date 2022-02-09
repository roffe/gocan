/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"github.com/spf13/cobra"
)

// cimCmd represents the cim command
var cimCmd = &cobra.Command{
	Use:   "cim",
	Short: "CIM related commands",
}

func init() {
	rootCmd.AddCommand(cimCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// cimCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// cimCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
