package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/roffe/gocan"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

const (
	DefaultCANRate = 500
)

var adapters = map[string]*gocan.AdapterConfig{
	"OBDLink EX": &gocan.AdapterConfig{
		Port:         "COM6",
		CANFilter:    []uint32{0x100},
		PortBaudrate: 2000000,
		CANRate:      DefaultCANRate,
	},
	"OBDLink SX": &gocan.AdapterConfig{
		Port:         "COM5",
		CANFilter:    []uint32{0x100},
		PortBaudrate: 2000000,
		CANRate:      DefaultCANRate,
	},
	"txbridge wifi": &gocan.AdapterConfig{
		CANRate: DefaultCANRate,
	},
	"CANlib #0 Kvaser Leaf Light v2": &gocan.AdapterConfig{
		CANRate: DefaultCANRate,
	},
	"CANUSB LW5ZEIRK": &gocan.AdapterConfig{
		CANRate: DefaultCANRate,
	},
	"CombiAdapter": &gocan.AdapterConfig{
		CANRate: DefaultCANRate,
	},
	"x64 J2534 #1 PCANPT32": &gocan.AdapterConfig{
		CANRate: DefaultCANRate,
	},
}

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	if len(os.Args) >= 2 && os.Args[1] == "list" {
		printAdapters(gocan.ListAdapters())
		return
	}

	var clientz []*gocan.Client

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapterOrder := []string{"OBDLink SX", "CANlib #0 Kvaser Leaf Light v2", "CombiAdapter", "CANUSB LW5ZEIRK" /*, "x64 J2534 #1 PCANPT32"*/}
	noAdapters := len(adapterOrder)
	log.Println("No of adapters used in test:", noAdapters)

	for i, name := range adapterOrder {
		cfg, ok := adapters[name]
		if !ok {
			log.Printf("Adapter %s not found", name)
			continue
		}

		adapter, err := gocan.NewAdapter(name, cfg)
		if err != nil {
			log.Printf("Failed to create adapter %s: %v", name, err)
			continue
		}
		log.Printf("conecting #%d: %s", i, name)
		c, err := gocan.NewWithAdapter(ctx, adapter)
		if err != nil {
			log.Printf("Failed to create client %s: %v", name, err)
			log.Fatalf("Failed to open %s: %v", name, err)
		}
		clientz = append(clientz, c)
	}

	for i, cl := range clientz {
		last := i == noAdapters-1
		createWorker(ctx, cl, i, last)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	errz := make(chan wrappedError, 20)
	for idx, c := range clientz {
		go createErrListener(ctx, c, idx, errz)
	}

outer:
	for {
		select {
		case err := <-errz:
			log.Printf("%s Client error: %v", adapterOrder[err.idx], err.err)
			if !gocan.IsRecoverable(err.err) {
				break outer
			}
		case <-ctx.Done():
			break outer
		case sig := <-sigChan:
			log.Printf("Got signal %v, stopping", sig)
			break outer
		}
	}

	cancel()
	for _, c := range clientz {
		log.Println("Closing client", c.Adapter().Name())
		c.Close()
	}

}

type wrappedError struct {
	idx int // index of the client
	err error
}

func createErrListener(ctx context.Context, cl *gocan.Client, idx int, errz chan<- wrappedError) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-cl.Err():
			if !ok {
				return
			}
			errz <- wrappedError{idx: idx, err: err}
		}
	}
}

func printAdapters(ad []gocan.AdapterInfo) {
	for _, adapter := range ad {
		log.Printf("Adapter: %s", adapter.Name)
		log.Printf("  Desc: %s", adapter.Description)
		log.Printf("  RequiresSerialPort: %v", adapter.RequiresSerialPort)
		log.Println("  Capabilities:")
		log.Printf("    HSCAN: %v", adapter.Capabilities.HSCAN)
		log.Printf("    SWCAN: %v", adapter.Capabilities.SWCAN)
		log.Printf("    KLine: %v", adapter.Capabilities.KLine)
		log.Println(strings.Repeat("-", 30))
	}
}

func createWorker(ctx context.Context, cl *gocan.Client, idx int, last bool) {
	if idx == 0 {
		go createProducer(ctx, cl, idx)
		return
	}
	go createChainer(ctx, cl, idx, last)
}

func createProducer(ctx context.Context, cl *gocan.Client, idx int) {
	t := time.NewTicker(100 * time.Millisecond)

	data := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x00, 0x00, 0x00}
	data2 := []byte{0xAB, 0xBA, 0x13, 0x37}

	defer t.Stop()
	tick := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick = !tick
			var dd []byte
			if tick {
				dd = data2
			} else {
				dd = data
			}
			frame := &gocan.CANFrame{
				Identifier: 0x101,
				Data:       dd,
				FrameType:  gocan.ResponseRequired,
			}
			log.Printf("#%d Frame: %v\n", idx, frame)
			start := time.Now()
			resp, err := cl.SendAndWait(ctx, frame, 2*time.Second, 0x100)
			if err != nil {
				log.Printf("Failed to send frame: %v", err)
				continue
			}
			log.Printf("#%d Frame: %v\n", idx, resp)
			log.Printf("RTT: %v\n", time.Since(start))
			log.Println(strings.Repeat("-", 30))
			if len(data) == 8 {
				if data[7] == 0xFF {
					data[6]++
					data[7] = 0x00
				} else {
					data[7]++
				}
			}
		}
	}
}

func createChainer(ctx context.Context, cl *gocan.Client, idx int, last bool) {
	nodeID := uint32(0x100 + idx)
	incoming := make(chan *gocan.CANFrame, 10)
	sub := cl.SubscribeChan(ctx, incoming, nodeID)
	defer sub.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-incoming:
			log.Printf("#%d Frame: %v\n", idx, frame)
			if !last {
				frame.Identifier = nodeID + 1
			} else {
				frame.Identifier = 0x100
			}
			frame.FrameType = gocan.Outgoing
			if err := cl.SendFrame(frame); err != nil {
				log.Printf("Failed to send frame: %v", err)
			}
			log.Printf("#%d Frame: %v\n", idx, frame)
		}
	}
}
