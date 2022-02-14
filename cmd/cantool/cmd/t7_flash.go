package cmd

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/roffe/gocan/pkg/t7"
	"github.com/spf13/cobra"
)

// ecuCmd represents the ecu command
var flashCmd = &cobra.Command{
	Use:   "flash <filename>",
	Short: "flash binary to ecu",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 900*time.Second)
		defer cancel()

		adapter, port, baudrate, err := getAdapterOpts()
		if err != nil {
			return err
		}
		c, err := initCAN(ctx, adapter, port, baudrate)
		if err != nil {
			return err
		}
		defer c.Close()

		tr := t7.New(c)

		filename := args[0]
		n, bin, err := tr.LoadBinFile(filename)
		if err != nil {
			return err
		}
		log.Printf("loaded %d bytes from %s\n", n, filepath.Base(filename))

		log.Println("\nECU Info:")
		if err := tr.Info(ctx); err != nil {
			log.Println("/!\\", err)
			return err
		}
		log.Println("Erase flash?")
		erase := yesNo()
		log.Println("Are you sure?")
		confirm := yesNo()

		if !confirm {
			return nil
		}

		if erase {
			if err := tr.Erase(ctx); err != nil {
				return err
			}
		}

		if err := tr.Flash(ctx, bin); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	t7Cmd.AddCommand(flashCmd)
}
