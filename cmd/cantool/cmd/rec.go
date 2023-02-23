package cmd

import (
	"log"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(recCmd)
}

var recCmd = &cobra.Command{
	Use:    "rec",
	Short:  "rec",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		ctx := cmd.Context()
		//c, err := initCAN(ctx, 0x649)

		dev, err := adapter.New(
			"j2534",
			&gocan.AdapterConfig{
				Port:         `C:\Program Files (x86)\Drew Technologies, Inc\J2534\MongoosePro GM II\monpa432.dll`,
				PortBaudrate: 0,
				CANRate:      33.3,
				CANFilter:    []uint32{0x64F, 0x649},
			},
		)

		if err != nil {
			return err
		}

		c, err := gocan.New(ctx, dev)
		if err != nil {
			return err
		}

		defer c.Close()

		deviceID := uint32(0x24f)
		responseID := uint32(0x64f)

		//leg := legion.New(c)
		gm := gmlan.New(c, deviceID, responseID)
		/*
			gm.TesterPresentNoResponseAllowed()

			if err := gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
				return err
			}
			defer func() {
				if err := gm.ReturnToNormalMode(ctx); err != nil {
					log.Println(err)
				}
			}()

			if err := gm.DisableNormalCommunication(ctx); err != nil {
				return err
			}
			/*
				if err := gm.RequestSecurityAccess(ctx, 0x0b, 5*time.Millisecond, calculateAccessKey); err != nil {
					return err
				}
		*/

		err = gm.WriteDataByIdentifier(ctx, 0x4d, []byte{0x8B, 0xF1, 0x6B, 0x6B, 0x40})
		if err != nil {
			return err
		}

		time.Sleep(10 * time.Millisecond)

		return nil
	},
}
