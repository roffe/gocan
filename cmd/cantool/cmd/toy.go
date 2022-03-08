package cmd

import (
	"log"

	"github.com/roffe/gocan/pkg/ecu/t8"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(toyCmd)
}

var toyCmd = &cobra.Command{
	Use:    "toy",
	Short:  "toy",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		defer c.Close()

		tr := t8.New(c)

		if err := tr.Bootstrap(ctx, infoCallback); err != nil {
			return err
		}

		b, err := tr.LegionIDemand(ctx, 0x02, 0x00)
		if err != nil {
			return err
		}

		log.Printf("%X", b)

		return nil
	},
}
