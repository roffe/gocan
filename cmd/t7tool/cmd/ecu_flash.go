package cmd

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/roffe/canusb/pkg/t7"
	"github.com/spf13/cobra"
)

// ecuCmd represents the ecu command
var flashCmd = &cobra.Command{
	Use:   "flash",
	Short: "flash <filename>",
	Long:  `Flash binary to ECU`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 900*time.Second)
		defer cancel()
		c, err := initCAN(ctx, 0x238, 0x258)
		if err != nil {
			return err
		}

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
	ecuCmd.AddCommand(flashCmd)
}
