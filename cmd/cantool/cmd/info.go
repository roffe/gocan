package cmd

import (
	"log"
	"time"

	"github.com/roffe/gocan/pkg/ecu"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "print ECU info",
	Args:  cobra.NoArgs,
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
		start := time.Now()
		if _, err := tr.Info(ctx, infoCallback); err != nil {
			return err
		}
		log.Println("took", time.Since(start).String())
		if err := tr.ResetECU(ctx, nil); err != nil {
			return err
		}

		return nil
	},
}

func infoCallback(v interface{}) {
	switch t := v.(type) {
	case string:
		log.Println(t)
	}
}
