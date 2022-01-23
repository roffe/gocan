package cmd

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/roffe/canusb"
	"github.com/roffe/canusb/cmd/t7tool/pkg/ui"
	"github.com/roffe/canusb/pkg/t7"
	"github.com/spf13/cobra"
)

func init() {
	canCMD.AddCommand(monitorCMD)
}

var filter = &ui.Input{
	Name:      "filter",
	Title:     "Filter",
	X:         0,
	Y:         15,
	W:         25,
	MaxLength: 30,
}

var mu sync.Mutex
var buffLines int64
var filters []uint32

var monitorCMD = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor the CANbus for frames",
	//Long:  `Flash binary to ECU`,
	Args: cobra.RangeArgs(0, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("Entering monitoring mode")
		ctx := cmd.Context()
		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		defer c.Close()
		//log.SetFlags(log.Ltime | log.Lmicroseconds)

		g, err := gocui.NewGui(gocui.OutputNormal)
		if err != nil {
			return err
		}
		g.Cursor = true
		defer g.Close()

		g.SetManagerFunc(layout)

		if err := initKeybindings(g, c); err != nil {
			return err
		}

		go frameParser(ctx, c, g)

		if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
			return err
		}

		return nil
	},
}

var frameCount = 0

func inFilters(identifier uint32) bool {
	mu.Lock()
	defer mu.Unlock()
	if len(filters) == 0 {
		return true
	}
	for _, id := range filters {
		if id == identifier {
			return true
		}
	}
	return false
}

var coolant int
var hPa uint16

func frameParser(ctx context.Context, c *canusb.Canusb, g *gocui.Gui) {
	msg := c.Subscribe(ctx)
	for og := range msg {
		if !inFilters(og.Identifier) {
			frameCount++
			continue
		}
		frameCount++
		if buffLines > 50000 {
			continue
		}

		f := *og // this must be here or else the pointer will get fucked
		var updateFunc = func(g *gocui.Gui) error {
			packets, err := g.View("packets")
			if err != nil {
				return err
			}

			sid, err := g.View("sid")
			if err != nil {
				return err
			}
			sid.Clear()

			if f.Identifier == 0x338 {
				fmt.Fprintf(sid, "%q\n", f.Data)
			}

			info, err := g.View("info")
			if err != nil {
				return err
			}

			info.Clear()
			fmt.Fprintf(info, "frames: %d\n", frameCount)
			fmt.Fprintf(info, "in buffer: %d\n", buffLines)
			fmt.Fprintln(info)
			fmt.Fprintf(info, "coolant: %d\n", coolant)
			fmt.Fprintf(info, "preassure hPa: %d\n", hPa)

			if f.Identifier == 0x5c0 {
				coolant = int(f.Data[1]) - 40
				b, err := hex.DecodeString(fmt.Sprintf("%02X%02X", f.Data[2], f.Data[3]))
				if err != nil {
					log.Fatal(err)
				}
				hPa = binary.LittleEndian.Uint16(b)
			}

			fmt.Fprintf(sid, "%X %d\n", f.Identifier, f.Identifier)
			fmt.Fprintf(packets, " %s || %s || %s\n", time.Now().Format("15:04:05.00000"), f.String(), t7.LookupID(f.Identifier))
			//packets.MoveCursor(0, 1)
			atomic.AddInt64(&buffLines, 1)
			return nil
		}

		frameCount++
		//	g.Update(updateInfo())
		g.Update(updateFunc)
	}
}

/*
func updateInfo() func(g *gocui.Gui) error {
	return func(g *gocui.Gui) error {
		info, err := g.View("info")
		if err != nil {
			return err
		}

		fmt.Fprintf(info, "frames: %d\n", frameCount)
		fmt.Fprintf(info, "in buffer: %d\n", buffLines)
		return nil
	}
}
*/
func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	if v, err := g.SetView("sid", 0, 0, 25, 4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = true
		v.Title = "SID"
	}
	if v, err := g.SetView("info", 0, 5, 25, 14); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = false
		v.Title = "Info"
	}

	if err := filter.Layout(g); err != nil {
		return err
	}

	if v, err := g.SetView("help", 0, maxY-21, 25, maxY-11); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = false
		v.Wrap = true
		v.Title = "Help"
		fmt.Fprintln(v, "<Q, Ctrl-C> Quit")
		fmt.Fprintln(v, "<Space> Autoscroll")
		fmt.Fprintln(v, "<Ctrl-F> Set filter")
	}

	if v, err := g.SetView("errors", 0, maxY-10, 25, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = false
		v.Wrap = true
		v.Title = "Errors"
	}

	if v, err := g.SetView("packets", 26, 0, maxX-1, maxY-1); err != nil { // maxX-41 inspect
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

func up(g *gocui.Gui, v *gocui.View) error {
	v.MoveCursor(0, -1, false)
	return nil
}

func down(g *gocui.Gui, v *gocui.View) error {
	v.MoveCursor(0, 1, false)
	return nil
}

func flipAutoscroll(g *gocui.Gui, v *gocui.View) error {
	v.Autoscroll = !v.Autoscroll
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}
func setFilter(c *canusb.Canusb) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		buff := strings.TrimRight(v.Buffer(), "\n")
		mu.Lock()
		defer mu.Unlock()
		if len(buff) == 0 {
			filters = []uint32{}
			if _, err := g.SetCurrentView("packets"); err != nil {
				return err
			}
			return nil
		}
		parts := strings.Split(buff, ",")
		filters = []uint32{}
		for _, p := range parts {
			if strings.HasPrefix(p, "0x") {
				p = strings.TrimLeft(p, "0x")
				hexString := fmt.Sprintf("%08s", p)
				b, err := hex.DecodeString(hexString)
				if err != nil {
					if v, errr := g.View("errors"); errr == nil {
						fmt.Fprintln(v, err)
					}
					return nil
				}
				parsed := binary.BigEndian.Uint32(b)
				filters = append(filters, parsed)
			} else {
				parsed, err := strconv.Atoi(p)
				if err != nil {
					if v, errr := g.View("errors"); errr == nil {
						fmt.Fprintln(v, err)
					}
					continue
				}
				filters = append(filters, uint32(parsed))
			}
		}

		code, mask := canusb.CalcAcceptanceFilters(filters...)
		c.Send(&canusb.RawCommand{Data: code})
		c.Send(&canusb.RawCommand{Data: mask})
		//c.Send(&canusb.RawCommand{Data: "O"})
		if v2, err2 := g.View("errors"); err2 == nil {
			fmt.Fprintf(v2, "Set %d %s %s\n", filters, code, mask)
		}
		if _, err := g.SetCurrentView("packets"); err != nil {
			return err
		}
		return nil
	}
}
func initKeybindings(g *gocui.Gui, c *canusb.Canusb) error {
	if err := g.SetKeybinding("", 'q', gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyCtrlF, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			if _, err := g.SetCurrentView("filter"); err != nil {
				return err
			}
			//v.Mask ^= '*'
			return nil
		}); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", '?', gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			if _, err := g.SetCurrentView("filter"); err != nil {
				return err
			}
			//v.Mask ^= '*'
			return nil
		}); err != nil {
		return err
	}

	if err := g.SetKeybinding("filter", gocui.KeyEnter, gocui.ModNone, setFilter(c)); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", 'c', gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			buffLines = 0
			v.Autoscroll = true
			v.Clear()
			v.SetOrigin(0, 0)
			//if vv, _ := g.View("inspect"); vv != nil {
			//	vv.Clear()
			//}
			return nil
		},
	); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyHome, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			cx, cy := v.Cursor()
			v.Autoscroll = false
			v.SetOrigin(0, 0)
			v.SetCursor(cx, cy)
			return nil
		},
	); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyEnd, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.Autoscroll = false
			cx, cy := v.Cursor()
			_, y := v.Size()
			v.SetOrigin(0, len(v.BufferLines())-y+1)
			v.SetCursor(cx, cy)
			return nil
		},
	); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeySpace, gocui.ModNone, flipAutoscroll); err != nil {
		return err
	}

	if err := g.SetKeybinding("packets", gocui.KeyArrowUp, gocui.ModNone, up); err != nil {
		return err
	}
	if err := g.SetKeybinding("packets", gocui.KeyArrowDown, gocui.ModNone, down); err != nil {
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
			v.MoveCursor(0, -10, false)
			return nil
		}); err != nil {
		return err
	}
	if err := g.SetKeybinding("packets", gocui.KeyPgdn, gocui.ModNone,
		func(g *gocui.Gui, v *gocui.View) error {
			v.MoveCursor(0, 10, false)
			return nil
		}); err != nil {
		return err
	}

	return nil
}
