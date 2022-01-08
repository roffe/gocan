package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"
)

var buffLines int
var monitorCMD = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor the CANbus for packages",
	//Long:  `Flash binary to ECU`,
	Args: cobra.RangeArgs(0, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("Entering monitoring mode")
		ctx := cmd.Context()
		c, err := initCAN(ctx, 0x1a0)
		if err != nil {
			return err
		}
		defer c.Close()
		//log.SetFlags(log.Ltime | log.Lmicroseconds)

		g, err := gocui.NewGui(gocui.Output256)
		if err != nil {
			return err
		}
		g.Cursor = true
		defer g.Close()

		g.SetManagerFunc(layout())

		if err := initKeybindings(g); err != nil {
			return err
		}

		go func() {
			frameCount := 0
			msg := c.Subscribe(ctx)
			for f := range msg {
				frameCount++
				g.Update(func(g *gocui.Gui) error {
					v, err := g.View("packets")
					if err != nil {
						return err
					}
					fmt.Fprintf(v, "%s> %s\n", time.Now().Format("15:04:05.00000"), f.String())
					buffLines++
					v2, err := g.View("info")
					if err != nil {
						return err
					}
					v2.Clear()
					fmt.Fprintf(v2, "frames: %d\n", frameCount)
					fmt.Fprintf(v2, "in buffer: %d\n", buffLines)

					return nil
				})
			}
		}()

		if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
			return err
		}

		return nil
	},
}

func init() {
	canCMD.AddCommand(monitorCMD)
}

func layout() func(*gocui.Gui) error {
	return func(g *gocui.Gui) error {
		maxX, maxY := g.Size()

		if v, err := g.SetView("info", 0, 0, 20, maxY-1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Autoscroll = false
			v.Overwrite = true
			v.Title = "Info"
			//fmt.Fprintln(v,"Frames: ")
			//if _, err := g.SetCurrentView("packets"); err != nil {
			//	return err
			//}
		}
		if v, err := g.SetView("packets", 21, 0, maxX-1, maxY-1); err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.SelFgColor = gocui.ColorCyan
			v.Autoscroll = true
			v.Highlight = true
			v.Title = "Frame view"

			if _, err := g.SetCurrentView("packets"); err != nil {
				return err
			}
		}
		return nil
	}
}

func initKeybindings(g *gocui.Gui) error {
	if err := g.SetKeybinding("", 'q', gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("packets", 'c', gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			buffLines = 0
			v.Autoscroll = true
			v.Clear()
			return nil
		},
	); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyHome, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.Autoscroll = false
			if err := v.SetOrigin(0, 0); err != nil {
				return err
			}
			v.SetCursor(0, 0)
			return nil
		},
	); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyEnd, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.Autoscroll = false
			_, y := g.Size()
			if err := v.SetOrigin(0, len(v.BufferLines())); err != nil {
				return err
			}
			v.SetCursor(0, y-1)
			return nil
		},
	); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeySpace, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.Autoscroll = !v.Autoscroll
			return nil
		},
	); err != nil {
		return err
	}
	if err := g.SetKeybinding("packets", gocui.KeyArrowUp, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.MoveCursor(0, -1, false)
			scrollView(v, -1)
			return nil
		}); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyArrowDown, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.MoveCursor(0, 1, false)
			scrollView(v, 1)
			return nil
		}); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyArrowLeft, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.MoveCursor(-1, 0, false)
			//scrollView(v, -1)
			return nil
		}); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyArrowRight, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.MoveCursor(1, 0, false)
			//scrollView(v, -1)
			return nil
		}); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyPgup, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			scrollView(v, -10)
			return nil
		}); err != nil {
		return err
	}
	if err := g.SetKeybinding("packets", gocui.KeyPgdn, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			scrollView(v, 10)
			return nil
		}); err != nil {
		return err
	}

	return nil
}

func scrollView(v *gocui.View, dy int) error {
	if v != nil {
		v.Autoscroll = false
		ox, oy := v.Origin()
		if err := v.SetOrigin(ox, oy+dy); err != nil {
			return err
		}
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
