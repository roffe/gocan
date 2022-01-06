/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"log"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var ecuCmd = &cobra.Command{
	Use:   "ecu",
	Short: "ECU related commands",
	Long:  `commands related to erasing, writing and dumping flash memory`,
	//Run: func(cmd *cobra.Command, args []string) {
	//	fmt.Println("ecu called")
	//},
}

func init() {
	rootCmd.AddCommand(ecuCmd)
}

func yesNo() bool {
	prompt := promptui.Select{
		Label:    "[Yes/No]",
		HideHelp: true,
		Items:    []string{"Yes", "No"},
	}
	_, result, err := prompt.Run()
	if err != nil {
		log.Fatalf("Prompt failed %v\n", err)
	}
	return result == "Yes"
}
