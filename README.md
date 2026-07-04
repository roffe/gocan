# goCAN

A Go CAN bus library for Linux, Windows and macOS with support for a wide
range of CAN adapters — from cheap ELM327/STN serial dongles to SocketCAN,
J2534 passthru devices, Kvaser, PCAN and more.

Linux maintainer wanted! Please contact me at gocan@roffe.nu if you want to help out.

## Installation

	go get github.com/roffe/gocan@latest

```go
import "github.com/roffe/gocan"
```

Some adapter backends need vendor libraries and are only compiled in when you
build with their tag (`go build -tags "combi,j2534"`):

| Build tag | Enables                        | Requires                    |
|-----------|--------------------------------|-----------------------------|
| `ftdi`    | d2xx based FTDI adapters       | FTDI D2XX driver            |
| `canlib`  | Kvaser Canlib                  | canlib32.dll                |
| `canusb`  | Lawicel CANUSB via DLL         | canusbdrv(64).dll           |
| `combi`   | CombiAdapter via libusb        | libusb-1.0.dll              |
| `j2534`   | J2534 passthru devices         | vendor J2534 DLL            |
| `pcan`    | PCAN-USB                       | PCANBasic.dll               |

Serial adapters (ELM327/STN/OBDLink, slcan, CANUSB in VCP mode, txbridge…) and
SocketCAN on Linux are always available without tags.

## Quick start

Adapters are looked up by name from a registry. `gocan.ListAdapterNames()`
returns everything compiled into your binary; names include e.g. `"ELM327"`,
`"CombiAdapterNew"`, `"txbridge wifi"` and on Linux one `"SocketCAN <dev>"`
entry per interface found.

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/roffe/gocan"
)

func main() {
	ctx := context.Background()

	c, err := gocan.New(ctx, "ELM327", &gocan.AdapterConfig{
		Port:         "COM3",   // or /dev/ttyUSB0
		PortBaudrate: 115200,   // serial adapters only
		CANRate:      500,      // CAN bit rate in kbit/s
		CANFilter:    []uint32{0x7E8}, // receive only these IDs (empty = everything)
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// Fire-and-forget send: queues the frame to the adapter
	if err := c.Send(0x7DF, []byte{0x02, 0x01, 0x0C, 0, 0, 0, 0, 0}, gocan.Outgoing); err != nil {
		log.Fatal(err)
	}

	// Wait for a single frame with one of the given IDs
	resp, err := c.Recv(ctx, 500*time.Millisecond, 0x7E8)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("id %03X data %X", resp.Identifier, resp.Data)
}
```

If you already have an `Adapter` instance (e.g. from `gocan.NewAdapter`) use
`gocan.NewWithOpts`, which also takes options such as `WithEventFunc` /
`WithLogger`:

```go
dev, _ := gocan.NewAdapter("ELM327", cfg)
c, err := gocan.NewWithOpts(ctx, dev, gocan.WithEventFunc(func(e gocan.Event) {
	log.Println(e.String())
}))
```

## Receiving frames

For a stream of frames instead of a one-shot `Recv`, create a subscription
filtered on zero or more CAN IDs (no IDs = all traffic):

```go
sub := c.Subscribe(ctx, 0x238, 0x258)
defer sub.Close()
for frame := range sub.Chan() {
	log.Printf("%03X %X", frame.Identifier, frame.Data)
}
```

`SubscribeFunc` runs a callback per frame instead, and `SubscribeChan` feeds a
channel you own:

```go
sub := c.SubscribeFunc(ctx, func(f *gocan.CANFrame) {
	log.Printf("%03X %X", f.Identifier, f.Data)
}, 0x238)
defer sub.Close()
```

## Request / response

`SendAndWait` sends a frame and blocks until a frame with one of the given
IDs arrives (or the timeout/context fires) — the everyday primitive for
ECU protocols:

```go
frame := gocan.NewFrame(0x005, []byte{0xC9, 0, 0, 0, 0, 0, 0, 0}, gocan.ResponseRequired)
resp, err := c.SendAndWait(ctx, frame, 1*time.Second, 0x00C)
```

### Frame types

Every frame carries a `CANFrameType` that tells the adapter how to treat it:

* `gocan.Outgoing` — fire and forget.
* `gocan.ResponseRequired` — tells buffered adapters (ELM/STN family) to wait
  for one response frame; use `ResponseRequiredWithResponses(n)` when a
  request yields several.
* `gocan.Incoming` — frames received from the bus.

### Synchronous sends

`Send` / `SendFrame` only queue a frame into the adapter's send buffer. When
you need to know the frame has actually been written to the hardware (e.g. to
pace frames against a slow ECU without guessing at sleeps), use `SendSync`:

```go
err := c.SendSync(ctx, frame, 100*time.Millisecond)
```

It blocks until the adapter confirms the write, the context is cancelled or
the timeout fires. Adapters that confirm write-completion report it via
`SupportsSync()`; on adapters that don't, `SendSync` degrades to a plain
`SendFrame`.

## Errors and lifecycle

The client owns the adapter: `c.Close()` shuts both down. If the adapter dies
on its own (unplugged USB, fatal driver error), the client's context is
cancelled — `c.Done()` / `c.Err()` / `c.Wait(ctx)` let you observe that:

```go
go func() {
	<-c.Done()
	log.Println("bus gone:", c.Err())
}()
```

Non-fatal adapter noise (status messages, recoverable errors) is delivered as
`Event`s via `WithEventFunc`/`WithEventChan` options or `c.OnEvent(fn)` at any
time.

## Writing your own adapter

Implement the `gocan.Adapter` interface (embed `gocan.BaseAdapter` for the
channel plumbing) and register it with `gocan.RegisterAdapter` from an
`init()`. If your send path can confirm that a frame has been written to the
hardware, construct with `NewSyncBaseAdapter` and call `frame.markSent()` on
every exit path of the send routine — that enables `SendSync` for your
adapter. `adapter_template.go` is a minimal starting point.

## Showcase

* [Saab CAN flasher](https://github.com/roffe/gocanflasher)
* [txlogger](https://github.com/roffe/txlogger)

## Supported Adapters

### USB Serial

* OBDLinx SX/EX/MX/MX+: https://www.obdlink.com/
* STN1130
* STN1170
* STN2120
* [CANUSB](https://www.canusb.com/products/canusb/) adapter running in VCP mode using [Lawicel ascii api](https://www.canusb.com/files/canusb_manual.pdf)
* CANable Nano and Pro 1.0 & 2.0 running [slcan](https://github.com/normaldotcom/canable-fw)
* [YACA](https://github.com/roffe/yaca)

### d2xx based FTDI adapters

these adapters can be accessible directly using the [d2xx api](https://ftdichip.com/wp-content/uploads/2023/09/D2XX_Programmers_Guide.pdf) from FTDI

* [OBDLink SX/EX](https://www.obdlink.com/)
* [Canusb adapter](https://www.canusb.com/)

### libusb

* CombiAdapter

### Canusb DLL

Supported via [goCANUSB](https://github.com/roffe/gocanusb)

Lawicel canusbdrv.dll for both 32 and 64bit is supported

### Kvaser Canlib

Supported via [goCANlib](https://github.com/roffe/gocanlib), Tested with the following adapters

* Kvaser Leaf Light V2 https://kvaser.com/

### J2534

Support for both 32 & 64bit DLL's. Your GOARCH will controll which DLL it will look for.

Do note that not all vendors provide 64bit DLL's so you migh need to build your software with GOARCH=386 to be able to use the j2534 DLL.
I've made a experimental CAN gateway that can be accessed over gRCP on linux or named pipes on windows to be able to use 32bit DLL's on 64bit systems. See [goCANGateway](https://github.com/roffe/gocangateway)

Most adapters that comes with a J2534 DLL will work. The list given is just ones verified to work.

#### Windows

* Drewtech Mongoose GM PRO II: https://www.drewtech.com/
* Scanmatik 2 PRO: https://scanmatik.pro/
* Tech2 passthru ( limited support )
* GM MDI
* OBDX Pro GT/VT: https://www.obdxpro.com/
* Kvaser Leaf Light V2: https://kvaser.com/
* PCAN-USB

#### Linux
* Tactrix Openport 2.0: https://github.com/dschultzca/j2534
* Machina: https://github.com/rnd-ash/Macchina-J2534
* WQCAN: https://github.com/witoldo7/STM32CAN

### Other

* SocketCAN, note. to run from user space set cap_net_admin for target application ```sudo setcap cap_net_admin=eip APPNAME```
* OBDX Pro GT BLE & Wifi
* Combiadapter via libusb
