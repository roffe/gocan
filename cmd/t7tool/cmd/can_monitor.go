package cmd

import (
	"fmt"
	"log"

	"github.com/jroimartin/gocui"
	"github.com/roffe/canusb"
	"github.com/spf13/cobra"
)

var monitorCMD = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor the CANbus for packages",
	//Long:  `Flash binary to ECU`,
	Args: cobra.RangeArgs(0, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("Entering monitoring mode")
		ctx := cmd.Context()
		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		msg := c.Subscribe(ctx)
		//log.SetFlags(log.Ltime | log.Lmicroseconds)

		g, err := gocui.NewGui(gocui.OutputNormal)
		if err != nil {
			log.Panicln(err)
		}
		defer g.Close()

		g.SetManagerFunc(layout(msg))

		if err := g.SetKeybinding("", gocui.KeyCtrlW, gocui.ModNone, quit); err != nil {
			log.Panicln(err)
		}

		if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
			log.Panicln(err)
		}

		return nil
	},
}

func init() {
	canCMD.AddCommand(monitorCMD)
}

func layout(c chan *canusb.Frame) func(*gocui.Gui) error {
	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()
		if v, err := g.SetView("packets", 0, 0, maxX, maxY-2); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			f := <-c
			fmt.Fprintln(v, f.String())
		}
		return nil
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
