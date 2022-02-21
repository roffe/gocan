package cmd

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/roffe/gocan/pkg/t5"
	"github.com/spf13/cobra"
)

// t5Cmd represents the t5 command
var t5sramDumpCmd = &cobra.Command{
	Use:   "dump <filename>",
	Short: "dump SRAM",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		defer c.Close()

		tr := t5.New(c)

		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}

		if err := tr.ResetECU(ctx); err != nil {
			return err
		}

		time.Sleep(1000 * time.Millisecond)

		// Cannot dump sram while in bootloader mode as we write bootloader to sram
		sram, err := tr.GetSRAMSnapshot(ctx)
		if err != nil {
			return err
		}

		if args[0] == "-" {
			pos := 0
			log.Println("--- SRAM dump ----------------")
			for _, b := range sram {
				fmt.Printf("%02X ", b)
				pos++
				if pos == 25 {
					fmt.Println()
					pos = 0
				}
			}
			fmt.Println()
			log.Println("------------------------------")
		} else {
			if err := os.WriteFile(args[0], sram, 0644); err != nil {
				log.Printf("failed to write sram file: %v", err)
			}
		}

		return nil
	},
}

func init() {
	t5sramCmd.AddCommand(t5sramDumpCmd)
}
