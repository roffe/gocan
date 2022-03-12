package cmd

import (
	"log"

	"github.com/roffe/gocan/pkg/ecu"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(dtcCmd)
}

var dtcCmd = &cobra.Command{
	Use:   "dtc",
	Short: "show ecu DTC's",
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

		dtcs, err := tr.ReadDTC(ctx)
		if err != nil {
			return err
		}

		if len(dtcs) == 0 {
			log.Println("No DTC's")
		} else {
			log.Println("Detected DTC's:")
		}

		for i, dtc := range dtcs {
			log.Printf("  #%d %s", i, dtc.String())
		}
		return nil
	},
}
