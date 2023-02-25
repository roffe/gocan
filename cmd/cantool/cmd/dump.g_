package cmd

import (
	"log"
	"os"

	"github.com/roffe/gocan/pkg/ecu"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(dumpCmd)
}

var dumpCmd = &cobra.Command{
	Use:   "dump <file>",
	Short: "dump ECU to file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		defer c.Close()

		tr, err := ecu.New(c, getECUType())
		if err != nil {
			return err
		}

		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}

		bin, err := tr.DumpECU(ctx, infoCallback)
		if err != nil {
			return err
		}

		if err := os.WriteFile(args[0], bin, 0644); err != nil {
			log.Printf("failed to write dump file: %v", err)
		}

		if err := tr.ResetECU(ctx, infoCallback); err != nil {
			return err
		}

		return nil
	},
}
