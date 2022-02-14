package cmd

import (
	"context"
	"log"
	"time"

	"github.com/roffe/gocan/pkg/t7"
	"github.com/spf13/cobra"
)

var readCMD = &cobra.Command{
	Use:   "dump <filename>",
	Short: "dump binary from ECU",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Minute)
		defer cancel()

		adapter, port, baudrate, err := getAdapterOpts()
		if err != nil {
			return err
		}
		c, err := initCAN(ctx, adapter, port, baudrate, 0x238, 0x258)
		if err != nil {
			return err
		}
		defer c.Close()

		tr := t7.New(c)
		log.Println("\nECU Info:")
		if err := tr.Info(ctx); err != nil {
			log.Println("/!\\", err)
			return err
		}
		log.Println("Continue?")
		confirm := yesNo()
		if !confirm {
			return nil
		}
		if err := tr.ReadECU(ctx, args[0]); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	t7Cmd.AddCommand(readCMD)
}
