# CANLang

CANLang is Lua scripting for GoCAN: request/response CAN flows written as
small scripts and attached to any GoCAN adapter, without recompiling the
host application. The language is standard Lua 5.1 (via
[gopher-lua](https://github.com/yuin/gopher-lua)) with two extra globals:
`bus`, connected to the CAN bus the host opened, and `sleep`.

```lua
-- pong.lua: echo every 0x100 frame back on 0x101
for f in bus:frames(0x100) do
	bus:send(0x101, f:bytes())
end
```

## Running scripts

With the bundled runner:

```
go run ./cmd/canlang -list                                  # adapters
go run ./cmd/canlang -adapter "SocketCAN vcan0" script.lua
go run ./cmd/canlang -adapter "CANUSB VCP" -port /dev/ttyUSB0 -rate 500 script.lua
```

Flags: `-adapter` (default `loopback`), `-port`, `-rate` in kbit/s (default
`500`), `-list`. Ctrl-C stops the script.

From Go, hand a script the bus you already have:

```go
import "github.com/roffe/gocan/v2/canlang"

err := canlang.Run(ctx, bus, "script.lua")
// or canlang.RunSource(ctx, bus, "name-for-errors", source)
```

`Run` blocks until the script returns, fails, or ctx is cancelled. Each call
runs an isolated Lua state on the calling goroutine; run several scripts by
calling `Run` on several goroutines — they can share one bus safely.

## The bus

### `bus:send(id, bytes)`

Sends one frame. `bytes` is a Lua table of up to 8 byte values; more than 8
is an error (classic CAN cannot carry it).

```lua
bus:send(0x240, {0x40, 0xA1, 0x02, 0x1A, 0x90})
```

### `bus:recv(timeout_ms, id...)` → `frame` | `nil, "timeout"`

Waits for the next frame carrying one of the given identifiers. No
identifiers means any frame.

```lua
local f, err = bus:recv(500, 0x258, 0x266)
if not f then print("nothing heard: " .. err) end
```

### `bus:request(id, bytes, timeout_ms, replyid...)` → `frame` | `nil, "timeout"`

Sends a frame and waits for a reply. The subscription is registered before
the send, so a fast responder cannot slip past.

```lua
local f, err = bus:request(0x100, {0x01, seq}, 1000, 0x101)
```

### `bus:frames(id...)` → iterator

Streams matching frames for a `for ... in` loop. The loop ends when the host
shuts down; delivery is buffered (64 frames) and non-blocking, so a script
that stops draining loses frames rather than stalling the bus.

```lua
for f in bus:frames(0x1A0, 0x280) do
	print(tostring(f))
end
```

### `sleep(ms)`

Pauses the script without holding anything up.

## Frames

Frames are read-only values:

| method | returns |
|---|---|
| `f:id()` | CAN identifier |
| `f:len()` | payload length, 0–8 |
| `f:u8(off)` | byte at offset |
| `f:u16(off)` | big-endian 16-bit value at offset |
| `f:bytes()` | payload as a Lua table |
| `f:hex()` | payload as hex text, e.g. `"01 A0 FF"` |
| `tostring(f)` | full one-line dump: id, length, hex, ASCII |

**Offsets are 0-based**, matching CAN documentation: `f:u8(0)` is the first
data byte, exactly like `data[0]` in a datasheet. Lua *tables* keep Lua's
1-based convention: `f:bytes()[1]` is the first byte, and the table passed
to `bus:send` starts at index 1. Reading out of range raises an error.

```lua
-- Trionic 7 0x1A0 broadcast: rpm is a big-endian u16 at bytes 1..2
for f in bus:frames(0x1A0) do
	print("rpm " .. f:u16(1) .. "  pedal " .. f:u8(5))
end
```

## Arguments

Values passed after the script name reach it as the standard Lua `arg`
table: `arg[0]` is the script name, `arg[1]`, `arg[2]`, … the caller's
arguments. Use `or` for a default.

```sh
canlang -adapter "CANUSB VCP" -port /dev/ttyUSB2 t7immo.lua 237OZG103863289
```

```lua
local hardwareNr = arg[1] or "237OZG103863289"
```

## Bits

Lua 5.1 has no bitwise operators (`&` and `>>` won't parse), so CANLang
ships a `bit` global in the LuaJIT style. Operations work on 32-bit unsigned
integers; inputs are normalized modulo 2^32, so `-1` means `0xFFFFFFFF`.

| function | result |
|---|---|
| `bit.band(x, ...)` | and |
| `bit.bor(x, ...)` | or |
| `bit.bxor(x, ...)` | xor |
| `bit.bnot(x)` | complement |
| `bit.lshift(x, n)`, `bit.rshift(x, n)` | logical shifts; `n` of 32+ gives 0, negative `n` shifts the other way |
| `bit.extract(x, field [, width])` | `width` bits (default 1) starting at bit `field` |
| `bit.btest(x, ...)` | `true` if `band(x, ...)` is nonzero |

```lua
-- T7 0x280 status bits
local b2 = f:u8(2)
local brake  = bit.extract(b2, 1)          -- one flag bit
local clutch = bit.btest(b2, 0x08)         -- same idea, as a boolean
local gear   = bit.extract(f:u8(1), 0, 4)  -- low nibble
```

## Errors and timeouts

Two kinds of failure, handled differently:

- **Timeouts are expected** — `recv` and `request` return `nil, "timeout"`
  and the script decides what to do.
- **Everything else is fatal** — the bus dying, the host cancelling, a
  malformed call. These raise a Lua error that unwinds the script; `Run`
  returns it to the host. Don't wrap loops in `pcall` to survive them: a
  script without a bus has nothing left to do.

## Gotchas

- Bitwise **operator syntax** still won't parse — that's Lua 5.3 grammar on
  a 5.1 VM. Use the [`bit` library](#bits) above.
- `bus:send` builds standard 11-bit frames. Received frames may be anything
  the adapter delivers.
- `print` writes to the host's stdout. The full Lua standard library is
  open — scripts are trusted, treat them like code.
- Every script runs in its own isolated Lua state: scripts cannot see each
  other's globals. The only thing concurrent scripts share is the CAN bus —
  which is exactly how two scripts talk to each other.

## A complete pair

`cmd/canlang/examples/` holds a runnable ping/pong pair: start each in its
own terminal on adapters connected to the same bus (or both on `vcan0`).

```lua
-- ping.lua
local seq = 0
while true do
	seq = (seq + 1) % 256
	local f, err = bus:request(0x100, {0x01, seq}, 1000, 0x101)
	if f then
		print("pong " .. f:u8(1) .. " [" .. f:hex() .. "]")
	else
		print("no pong: " .. err)
	end
	sleep(500)
end
```

```lua
-- pong.lua
for f in bus:frames(0x100) do
	bus:send(0x101, f:bytes())
end
```
