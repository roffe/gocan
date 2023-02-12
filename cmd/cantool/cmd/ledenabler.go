/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/spf13/cobra"
)

// ledenablerCmd represents the ledenabler command
var ledenablerCmd = &cobra.Command{
	Use:   "ledenabler",
	Short: "led enabler for 9-3",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
		defer cancel()

		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		defer c.Close()

		gm := gmlan.New(c, 0x24F, 0x64F)
		/*
			f, err := gm.ReadDataByIdentifier(ctx, 0x45) // Read REC Bulb Outage &Substitution Lighting
			if err != nil {
				return err
			}
			log.Println("BO REC", string(f[:]))
		*/
		f0, err := gm.ReadDataByIdentifier(ctx, 0x40) // Read PWM Settings for Bulbs
		if err != nil {
			return err
		}
		log.Println("PWM UEC", string(f0[:]))

		f2, err := gm.ReadDataByIdentifier(ctx, 0x4D) // Read UHEC Bulb Outage & Substitution
		if err != nil {
			return err
		}
		log.Println("BO UEC", string(f2[:]))

		prompt := promptui.Select{
			Label: "Disable Bulb Outage [Yes/No]",
			Items: []string{"Yes", "No"},
		}
		_, result, err := prompt.Run()
		if err != nil {
			return fmt.Errorf("prompt failed %v", err)
		}

		if result == "Yes" {
			err := gm.WriteDataByIdentifier(ctx, 0x45, []byte{0x00}) // set Bulb Outage & Substitution Lighting to 0?
			if err != nil {
				return errors.New("failed to disable bulb outage in REC ")
			}
			err = gm.WriteDataByIdentifier(ctx, 0x4F, []byte{0x00}) // set Bulb Outage & Substitution to 0?
			if err != nil {
				return errors.New("failed to disable bulb outage in UEC ")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(ledenablerCmd)
}
