# goCAN

A Go CAN bus library for Linux, Windows and macOS with support for a wide
range of CAN adapters — from cheap ELM327/STN serial dongles to SocketCAN,
J2534 passthru devices, Kvaser, PCAN and more.

Linux maintainer wanted! Please contact me at gocan@roffe.nu if you want to help out.

## Installation

The current API is v2:

	go get github.com/roffe/gocan/v2@latest

```go
import gocan "github.com/roffe/gocan/v2"
```

The root package `github.com/roffe/gocan` is the legacy v1 API. For existing
v1 code, see [v2/MIGRATION.md](v2/MIGRATION.md).

Adapters register themselves on import. Native adapters live under
`github.com/roffe/gocan/v2/adapters/...`; import the ones you need, or use
`adapters/all` in applications that list adapters dynamically.

Some adapter backends still need vendor libraries and are only compiled in when
you build with their tag (`go build -tags "canlib,j2534"`):

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

The simplest working example uses the built-in loopback adapter. This mirrors
the tested example in [v2/example_test.go](v2/example_test.go).

```go
package main

import (
	"context"
	"fmt"

	gocan "github.com/roffe/gocan/v2"
)

func main() {
	ctx := context.Background()

	bus, err := gocan.Open(ctx, "loopback", gocan.Config{})
	if err != nil {
		panic(err)
	}
	defer bus.Close()

	reply, err := bus.Request(ctx, gocan.NewFrame(0x123, []byte("hello")), 0x123)
	if err != nil {
		panic(err)
	}
	fmt.Printf("0x%03X %s\n", reply.ID, reply.Bytes())
}
```

For real hardware, import the adapter package you want and open it by its
registered name:

```go
import (
	gocan "github.com/roffe/gocan/v2"
	_ "github.com/roffe/gocan/v2/adapters/canusb"
)

bus, err := gocan.Open(ctx, "CANUSB VCP", gocan.Config{
	Port:    "/dev/ttyUSB0",
	CANRate: 500,
})
```

`gocan.Adapters()` and `gocan.AdapterNames()` return everything registered in
your binary.

## Receiving frames

Use `Recv` for one frame, `Subscribe` for a channel, or `Frames` for an
iterator-style loop. Timeouts are handled with `context.WithTimeout`.

```go
frame, err := bus.Recv(ctx, 0x258)
ch := bus.Subscribe(ctx, 0x238, 0x258)
for frame := range bus.Frames(ctx, 0x1A0, 0x280) {
	fmt.Printf("%03X % X\n", frame.ID, frame.Bytes())
}
```

## Request / response

`Request` sends a frame and waits for a reply with one of the given CAN IDs.
For buffered adapters, multi-frame reply expectations are stamped on the
context with `gocan.WithExpectedResponses`.

```go
rctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
defer cancel()

reply, err := bus.Request(rctx, gocan.NewFrame(0x240, data), 0x258, 0x266)
```

`Send` returns when the adapter has written the frame, so there is no separate
`SendSync` API in v2.

## Errors and lifecycle

The bus owns the adapter: `bus.Close()` shuts both down. If the adapter dies on
its own (unplugged USB, fatal driver error), the bus context is cancelled.
Use `bus.Done()`, `bus.Err()` and `bus.Wait(ctx)` to observe that lifecycle.

```go
go func() {
	<-bus.Done()
	fmt.Println("bus gone:", bus.Err())
}()
```

Non-fatal adapter noise (status messages, recoverable errors) is delivered as
`Event`s via `gocan.WithEventFunc`, `gocan.WithLogger`, or `bus.OnEvent(...)`.

## Writing your own adapter

Implement the `gocan.Adapter` interface and register it from an `init()`.
Native v2 adapters live under [v2/adapters](v2/adapters), and
[v2/loopback.go](v2/loopback.go) is the minimal reference implementation.

```go
type Adapter interface {
	Open(ctx context.Context, bus *Bus) error
	Send(ctx context.Context, f Frame) error
	Close() error
}
```

Incoming traffic is pushed back into the bus with `bus.Deliver(frame)`.
Recoverable problems are emitted with `bus.Emit(...)`, and fatal adapter
failures terminate the bus with `bus.Fatal(err)`.

## Legacy v1

The repository still contains the v1 API at `github.com/roffe/gocan`, but new
development should target `github.com/roffe/gocan/v2`. If you are upgrading an
existing client, start with [v2/MIGRATION.md](v2/MIGRATION.md).

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
