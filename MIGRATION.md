# Migrating from GoCAN v1 to v2

v2 removes the channel plumbing, hidden frame state and duplicate timeout
mechanisms of v1. This document maps every v1 concept to its v2 replacement.

```go
// v1
import "github.com/roffe/gocan"

// v2
import gocan "github.com/roffe/gocan/v2"
```

Until v2 is tagged, `v2/go.mod` carries a `replace github.com/roffe/gocan =>
../` for local development; drop it once v1 is tagged and pushed.

## Package layout

v1 put everything — core and all ~30 adapters — in one package, so every
consumer inherited every adapter's dependencies (libusb/cgo, grpc, …). In v2
the core package is pure stdlib and adapters register themselves on import,
like `database/sql` drivers:

| Package                                  | Contents                                       |
|------------------------------------------|------------------------------------------------|
| `github.com/roffe/gocan/v2`              | core: `Bus`, `Frame`, registry, `loopback`     |
| `github.com/roffe/gocan/v2/adapters/...` | native adapters, one package each              |
| `github.com/roffe/gocan/v2/adapters/all` | blank-imports every native adapter             |

Import the adapters you use (or `adapters/all` for a GUI listing everything)
and open them by registry name as before. Every v1 adapter has a native v2
port; opt-in adapters that need vendor SDKs keep their v1 build tags
(`canlib`, `ftdi`, `j2534`, `pcan`, `rcan`).

## API mapping

| v1                                            | v2                                                  |
|-----------------------------------------------|-----------------------------------------------------|
| `gocan.New(ctx, name, &cfg)`                  | `gocan.Open(ctx, name, cfg)`                        |
| `gocan.NewWithOpts(ctx, adapter, opts...)`    | `gocan.OpenAdapter(ctx, adapter, opts...)`          |
| `gocan.NewAdapter(name, &cfg)`                | `gocan.NewAdapter(name, cfg)` + `OpenAdapter` later |
| `*gocan.Client`                               | `*gocan.Bus`                                        |
| `*gocan.CANFrame`                             | `gocan.Frame` (plain value)                         |
| `gocan.NewFrame(id, data, gocan.Outgoing)`    | `gocan.NewFrame(id, data)`                          |
| `frame.Identifier`                            | `frame.ID`                                          |
| `frame.RTR`                                   | `frame.Remote`                                      |
| `frame.DLC()` / `len(frame.Data)`             | `frame.Length` / `frame.Bytes()`                    |
| `frame.FrameType`, `gocan.ResponseRequired`   | ctx hint (see [Frame types](#frame-types-are-gone)) |
| `gocan.ResponseRequiredWithResponses(n)`      | `gocan.WithExpectedResponses(ctx, n)`               |
| `frame.Timeout` (buffered-adapter hint)       | the ctx deadline                                    |
| `c.SendFrame(f)`                              | `bus.Send(ctx, f)`                                  |
| `c.Send(id, data, frameType)`                 | `bus.Send(ctx, gocan.NewFrame(id, data))`           |
| `c.SendExtended(id, data, frameType)`         | `bus.Send(ctx, gocan.NewExtendedFrame(id, data))`   |
| `c.SendSync(ctx, f, timeout)`                 | `bus.Send(ctx, f)` — sends always confirm the write |
| `c.SendAndWait(ctx, f, timeout, ids...)`      | `bus.Request(ctx, f, ids...)`                       |
| `c.Recv(ctx, timeout, ids...)`                | `bus.Recv(ctx, ids...)`                             |
| `c.Subscribe(ctx, ids...)` (`*Subscriber`)    | `bus.Subscribe(ctx, ids...)` (`<-chan Frame`)       |
| `sub.Chan()`                                  | the channel itself                                  |
| `sub.Close()`                                 | cancel the ctx given to `Subscribe`                 |
| `c.SubscribeChan(ctx, ch, ids...)`            | gone — drain the returned channel into your own     |
| `c.SubscribeFunc(ctx, fn, ids...)`            | `for f := range bus.Frames(ctx, ids...) { fn(f) }`  |
| `gocan.WithEventHandler` / `WithEventFunc`    | `gocan.WithEventFunc`                               |
| `gocan.WithEventChan(ch)`                     | `bus.OnEvent` + your own channel                    |
| `gocan.WithLogger(l)`                         | `gocan.WithLogger(l)` (unchanged)                   |
| `c.OnEvent` / `Wait` / `Err` / `Done` / `Context` / `Close` | unchanged on `Bus`                    |
| `c.Adapter()` / `c.AdapterName()`             | unchanged on `Bus`                                  |
| `gocan.AdapterConfig`                         | `gocan.Config` (by value)                           |
| `cfg.AdditionalConfig`                        | `cfg.Extra`                                         |
| `cfg.PrintVersion`                            | gone — version info arrives as info events          |
| `gocan.RegisterAdapter(&info)`                | `gocan.Register(info)`                              |
| `gocan.ListAdapters()` / `ListAdapterNames()` | `gocan.Adapters()` / `gocan.AdapterNames()`         |
| `gocan.SystemMsg*` identifiers                | gone (see [System messages](#system-messages-are-gone)) |
| `gocan.TimeoutError`                          | gone — context errors (`context.DeadlineExceeded`)  |
| `gocan.ErrClosed`                             | `gocan.ErrClosed` (unchanged)                       |

## Timeouts: contexts only

Every v1 call that took both a context and a timeout now takes only the
context. Wrap with `context.WithTimeout` where you previously passed a
duration:

```go
// v1
frame, err := c.SendAndWait(ctx, frame, 150*time.Millisecond, 0x258)

// v2
rctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
defer cancel()
frame, err := bus.Request(rctx, frame, 0x258)
```

Timeouts surface as `context.DeadlineExceeded`, not `gocan.TimeoutError`.

## Frames are values

`Frame` carries no per-send state, so the v1 rules — "frames are single-use",
"received frames are read-only", "don't resend a frame" — are all gone. Build
one frame and send it in a loop if you like.

```go
// v1
frame := gocan.NewFrame(0x240, data, gocan.Outgoing) // *CANFrame, single-use

// v2
frame := gocan.NewFrame(0x240, data) // Frame value, reusable
```

Received frames arrive as values too; mutating your copy affects nobody else.
`Data` is a fixed `[8]byte` with `Length` saying how much is valid — use
`frame.Bytes()` where you used `frame.Data` before.

## Frame types are gone

`CANFrameType` (`Incoming` / `Outgoing` / `ResponseRequired`) existed for the
ELM/STN adapter family, which wants to know how many reply frames to wait
for. That hint no longer travels on every frame in the system — it is
request-scoped, so it rides on the context:

```go
// v1
frame := gocan.NewFrame(0x7E0, data, gocan.ResponseRequiredWithResponses(4))
resp, err := c.SendAndWait(ctx, frame, 300*time.Millisecond, 0x7E8)

// v2
rctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
defer cancel()
rctx = gocan.WithExpectedResponses(rctx, 4) // Request defaults to 1 without this
resp, err := bus.Request(rctx, gocan.NewFrame(0x7E0, data), 0x7E8)
```

`Request` stamps a hint of 1 automatically, so single-reply exchanges need
nothing. Plain `Send` without a hint is fire-and-forget (v1 `Outgoing`).
Adapters read the hint with `gocan.ExpectedResponses(ctx)`; the ones that
don't buffer replies simply never look.

v1's per-command `timeout` variable became `gocan.WithResponseTimeout(ctx, d)`
— the on-wire reply wait for one exchange. Buffered adapters default to
250 ms without it, and `Request` stamps it from its context deadline when
that deadline is near (≤ 10 s). A far deadline is never used: an
operation-lifetime context (a 20-minute dump) says nothing about how long
one frame's reply takes.

To collect a multi-frame response, subscribe before sending:

```go
ch := bus.Subscribe(rctx, 0x7E8)
if err := bus.Send(gocan.WithExpectedResponses(rctx, 4), frame); err != nil { ... }
for i := 0; i < 4; i++ {
	f, ok := <-ch
	...
}
```

## System messages are gone

v1 smuggled adapter commands and out-of-band data through reserved CAN IDs
(`SystemMsg*`). In v2:

- Adapter commands are methods on the concrete adapter type (e.g. the CANUSB
  adapter's `SetFilter`), reached via `bus.Adapter()` and a type assertion.
- Out-of-band notifications are `Event`s — subscribe with `bus.OnEvent`.

Adapters with a real host-side protocol expose proper methods instead —
e.g. the txbridge adapter's `Command`/`Raw`/`Subscribe`/`Request` over
`pkg/serialcommand`, or the CombiAdapter's `GetADCValue`.

## Subscriptions

`Subscriber` no longer exists. `Subscribe` returns a plain `<-chan Frame`
whose lifetime is the context you pass; `Frames` wraps it as an iterator.

```go
// v1
sub := c.Subscribe(ctx, 0x1A0)
defer sub.Close()
for frame := range sub.Chan() { ... }

// v2
for frame := range bus.Frames(ctx, 0x1A0) { ... }

// v2, in a select loop
ch := bus.Subscribe(ctx, 0x1A0)
for {
	select {
	case f, ok := <-ch:
		if !ok { return } // ctx cancelled or bus terminated
		...
	case <-other:
	}
}
```

The channel is closed for you when the context is cancelled or the bus dies —
no `Close` call, no leaked subscriptions. Delivery remains non-blocking, like
v1: slow consumers lose frames and a warning event is emitted.

## Adapter names

Use `gocan.AdapterName(a)` for a human-readable adapter name before a bus
exists: it reports the registry name the adapter was constructed under
(`NewAdapter`/`Open`), then an optional `Name() string` on the concrete
type, then the Go type name. `Bus.AdapterName()` reports the same after
opening. (The v1→v2 transition shim, `compat`, was removed once every
adapter had a native port.)

## Porting an adapter natively

The v1 adapter contract was four channels (`Send`/`Recv`/`Err`/`Event`) plus
`Open`/`Close`/`Name`. The v2 contract is three methods:

```go
type Adapter interface {
	Open(ctx context.Context, bus *Bus) error
	Send(ctx context.Context, f Frame) error
	Close() error
}
```

Mechanical translation:

| v1 pattern                                  | v2 pattern                          |
|---------------------------------------------|-------------------------------------|
| read loop: `recvChan <- frame`              | `bus.Deliver(frame)`                |
| send goroutine draining `sendChan`          | gone — write directly in `Send`; the bus serializes callers |
| `frame.markSent()`                          | gone — returning from `Send` is the confirmation |
| `errChan <- err` (fatal)                    | `bus.Fatal(err)`                    |
| `evtChan <- Event{...}` / `ba.Error(...)`   | `bus.Emit(Event{...})`              |
| `BaseAdapter`, `closeChan`                  | gone — watch the `ctx` given to `Open` |
| `Name()`                                    | gone — the registry name is used    |

Shutdown ordering is defined for you: `Bus.Close` cancels the bus context
*before* calling the adapter's `Close`. So in a read loop, an I/O error while
`ctx.Err() == nil` means the port died (`bus.Fatal`); an error after `ctx` is
done is a normal shutdown (just return). Don't nil the port field in `Close`
— the read loop may still be touching it.

Native adapters live in their own package under `adapters/` and register
themselves in `init()`. [adapters/canusb](adapters/canusb/canusb.go) is the
reference port, and its test file shows how to test an adapter against a
fake port without hardware.

## Not carried over

- `SubscribeChan` (bring-your-own-channel): drain `Subscribe`'s channel into
  your own if you need custom buffering.
- `gocan.TimeoutError`, `ErrResponseChannelClosed`: context errors and
  closed channels replace them.
- The v1 CLI and gRPC gateway server: still v1-only. The protocol-layer
  packages (`pkg/serialcommand`, `pkg/dvi`, SDK bindings, `gmlan`) and the
  gRPC `proto` client types were copied into `v2/pkg/...`, `v2/gmlan` and
  `v2/proto`.
