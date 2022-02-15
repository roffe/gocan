package cmd

import (
	"github.com/spf13/cobra"
)

// t5Cmd represents the t5 command
var t5Cmd = &cobra.Command{
	Use:   "t5",
	Short: "Trionic 5 ECU related commands",
	Long:  `commands related to erasing, writing and dumping flash memory`,
}

func init() {
	rootCmd.AddCommand(t5Cmd)
}
