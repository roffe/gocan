# GoCAN v2

A CAN bus library for Go with pluggable hardware adapters.

v2 is a redesign of [GoCAN](https://github.com/roffe/gocan) with a smaller,
more idiomatic API. Migrating from v1? See [MIGRATION.md](MIGRATION.md).

```
go get github.com/roffe/gocan/v2
```

## Quick start

```go
import (
	gocan "github.com/roffe/gocan/v2"
	_ "github.com/roffe/gocan/v2/adapters/canusb" // registers "CANUSB VCP"
)

bus, err := gocan.Open(ctx, "CANUSB VCP", gocan.Config{
	Port:    "/dev/ttyUSB0",
	CANRate: 500,
})
if err != nil {
	return err
}
defer bus.Close()

// Fire and forget
err = bus.Send(ctx, gocan.NewFrame(0x240, []byte{0x3F, 0x81}))

// Request / reply, bounded by the context
ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
defer cancel()
reply, err := bus.Request(ctx, gocan.NewFrame(0x240, data), 0x258, 0x266)

// Stream frames
for frame := range bus.Frames(ctx, 0x1A0, 0x280) {
	fmt.Println(frame)
}
```

## Design

- **The core is dependency-free.** `github.com/roffe/gocan/v2` is pure
  stdlib. Adapters live in their own packages (`adapters/<name>`) that
  register themselves on import, like `database/sql` drivers — you compile
  and link only the hardware support you actually import.
- **`Frame` is a plain value.** 16 bytes, no pointers, no hidden state. Copy
  it, reuse it, share it between goroutines freely.
- **Contexts are the only timeout mechanism.** No timeout parameters, no
  library-specific timeout errors. `context.WithTimeout` bounds `Recv`,
  `Request` and friends; cancelling a subscription's context ends it.
- **Adapters are three methods.** `Open`, `Send`, `Close`. Incoming traffic
  is pushed to the bus (`bus.Deliver`), notifications are events
  (`bus.Emit`), and an unrecoverable failure (`bus.Fatal`) terminates the
  bus. No channel plumbing.
- **`Send` confirms the write.** The bus serializes senders and an adapter
  returns from `Send` once the frame reached the hardware, giving natural
  inter-frame pacing without a separate sync API.

## Receiving frames

```go
// One frame, any of the given IDs (empty = any frame at all)
frame, err := bus.Recv(ctx, 0x258)

// Channel, for use in a select loop. Closed when ctx ends or the bus dies.
ch := bus.Subscribe(ctx, 0x258)

// Iterator over the same thing
for frame := range bus.Frames(ctx, 0x258) { ... }
```

Delivery is non-blocking: a subscriber that stops draining loses frames (and
a warning event tells you so).

## Events and lifecycle

```go
bus, err := gocan.Open(ctx, name, cfg,
	gocan.WithLogger(slog.Default()),          // forward events to slog
	gocan.WithEventFunc(func(e gocan.Event) {  // or handle them yourself
		fmt.Println(e)
	}),
)

stop := bus.OnEvent(func(e gocan.Event) { ... }) // add/remove at runtime
defer stop()

err = bus.Wait(ctx) // block until the bus dies; returns the fatal error, if any
```

`bus.Done()` / `bus.Err()` / `bus.Context()` expose the same lifecycle for
select loops. A clean `Close` reports `nil` from `Err`.

## Adapters

Adapters register themselves when their package is imported. Native v2
adapters:

| Name         | Import                                     | Description                               |
|--------------|--------------------------------------------|-------------------------------------------|
| `CANUSB VCP` | `github.com/roffe/gocan/v2/adapters/canusb` | Lawicel CANUSB over FTDI virtual COM port |
| `loopback`   | built into the core                         | Virtual echo adapter for tests            |

`adapters/all` blank-imports every native adapter, for GUI apps that list
them at runtime.

`gocan.Adapters()` / `gocan.AdapterNames()` list what is registered, with
descriptions and capabilities for building UIs.

## Scripting (CANLang)

[canlang](canlang/) embeds Lua so request/response flows can be scripted
against a bus without recompiling the host — see its
[README](canlang/README.md) for the script API. Like the adapters, it is a
separate package: the core stays dependency-free unless you import it.

```lua
for f in bus:frames(0x1A0) do
	print("rpm " .. f:u16(1))
end
```

Run scripts standalone with [cmd/canlang](cmd/canlang/main.go), or embed
with `canlang.Run(ctx, bus, "script.lua")`.

## Writing an adapter

Implement three methods and register a constructor from your own package.
The [loopback](loopback.go) adapter is the minimal example;
[adapters/canusb](adapters/canusb/canusb.go) is a complete serial adapter
with a reply parser and hardware-free tests.

```go
type Adapter interface {
	Open(ctx context.Context, bus *Bus) error // start; push frames via bus.Deliver
	Send(ctx context.Context, f Frame) error  // write one frame; never called concurrently
	Close() error
}

func init() {
	gocan.Register(gocan.AdapterInfo{
		Name: "my adapter",
		New:  func(cfg gocan.Config) (gocan.Adapter, error) { ... },
	})
}
```

Rules of thumb:

- Goroutines started in `Open` should exit when `ctx` is done.
- A read error while `ctx` is still alive is a dead port: call `bus.Fatal`.
  If `ctx` is already done it's a shutdown: just return.
- Recoverable trouble is an event (`bus.Emit`), not an error return.
