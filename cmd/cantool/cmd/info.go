package cmd

import (
	"log"
	"math"
	"time"

	"github.com/roffe/gocan/pkg/bar"
	"github.com/roffe/gocan/pkg/ecu"
	"github.com/schollz/progressbar/v3"
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
		headers, err := tr.Info(ctx, infoCallback)
		if err != nil {
			return err
		}

		for _, h := range headers {
			log.Printf("%-27s : %s", h.Desc, h.Value)
		}

		log.Println("Done, took", time.Since(start).String())
		if err := tr.ResetECU(ctx, infoCallback); err != nil {
			return err
		}

		return nil
	},
}
var max float64
var ask *progressbar.ProgressBar

func infoCallback(v interface{}) {
	switch t := v.(type) {
	case string:
		log.Println(t)
	case float64:
		if t < 0 {
			mm := math.Abs(t)
			ask = bar.New(int(mm), "Progress")
			max = mm
			return
		}
		ask.Set(int(t))
		if t == max {
			if ask != nil {
				ask.Close()
				log.Println("")
				//ask = nil
			}
		}
	}
}
