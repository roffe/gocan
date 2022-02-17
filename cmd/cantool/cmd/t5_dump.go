package cmd

import (
	"log"
	"os"

	"github.com/roffe/gocan/pkg/t5"
	"github.com/spf13/cobra"
)

// t5Cmd represents the t5 command
var t5dumpCmd = &cobra.Command{
	Use:   "dump <file>",
	Short: "dump ECU to file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx, 0xC)
		if err != nil {
			return err
		}
		defer c.Close()

		tr := t5.New(c)

		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}

		bin, err := tr.DumpECU(ctx)
		if err != nil {
			return err
		}

		if err := os.WriteFile(args[0], bin, 0644); err != nil {
			log.Printf("failed to write dump file: %v", err)
		}

		if err := tr.ResetECU(ctx); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	t5Cmd.AddCommand(t5dumpCmd)
}
