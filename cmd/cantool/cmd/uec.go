package cmd

import (
	"log"

	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(uecCmd)
}

var uecCmd = &cobra.Command{
	Use:    "uec",
	Short:  "uec",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		ctx := cmd.Context()
		c, err := initCAN(ctx, 0x54F, 0x64F)
		if err != nil {
			return err
		}
		defer c.Close()

		deviceID := uint32(0x24F)
		responseID := uint32(0x64F)

		//leg := legion.New(c)
		gm := gmlan.New(c, deviceID, responseID, 0x54F)

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

		pwmSettings, err := gm.ReadDataByIdentifier(ctx, 0x40)
		if err != nil {
			return err
		}
		log.Printf("PWM Settings for Bulbs: %X", pwmSettings)

		bulbOutage, err := gm.ReadDataByIdentifier(ctx, 0x4D)
		if err != nil {
			return err
		}
		log.Printf("Bulb Outage & Substitution: %X", bulbOutage)

		return nil
	},
}

func convertSeedCIM(seed int) int {
	//log.Printf("converting seed: 0x%03X\n", seed)
	key := (seed + 0x9130) & 0xFFFF
	key = (key >> 8) | (key << 8)
	return (0x3FC7 - key) & 0xFFFF
}

func calculateAccessKey(seed []byte, level byte) (byte, byte) {
	val := int(seed[0])<<8 | int(seed[1])

	key := convertSeedCIM(val)

	switch level {
	case 0xFB:
		key ^= 0x8749
		key += 0x06D3
		key ^= 0xCFDF
	case 0xFD:
		key /= 3
		key ^= 0x8749
		key += 0x0ACF
		key ^= 0x81BF
	}

	return (byte)((key >> 8) & 0xFF), (byte)(key & 0xFF)
}

func convertSeed(seed int) int {
	key := seed>>5 | seed<<11
	return (key + 0xB988) & 0xFFFF
}
