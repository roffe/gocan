// Package canlang embeds a Lua interpreter (gopher-lua) bound to a gocan
// Bus, so request/response CAN flows can be scripted without recompiling the
// host application.
//
// A script runs with three globals:
//
//	bus    the CAN bus
//	  bus:send(id, {bytes})                     send one frame
//	  bus:recv(timeout_ms, id...)               wait for a frame; frame or nil,"timeout"
//	  bus:request(id, {bytes}, timeout_ms, id...) send and wait for reply
//	  bus:frames(id...)                         iterator: for f in bus:frames(0x100) do ... end
//	sleep(ms)
//	bit    32-bit bitwise ops (Lua 5.1 has no operators): band, bor, bxor,
//	       bnot, lshift, rshift, extract, btest
//	arg    command-line args: arg[0]=script name, arg[1..n]=caller args
//
// Frames are read-only userdata: f:id(), f:len(), f:u8(off), f:u16(off)
// (big-endian), f:bytes() (1-based table), f:hex(), tostring(f). Byte
// offsets are 0-based, matching CAN documentation.
//
// Each script owns its LState and must stay on one goroutine; run one script
// per Run call. Scripts are trusted: the full Lua stdlib is open.
package canlang

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gocan "github.com/roffe/gocan/v2"
	lua "github.com/yuin/gopher-lua"
)

// Run executes the Lua script at path against bus, blocking until the script
// returns, fails, or ctx is cancelled. args are exposed to the script as the
// standard Lua arg table (arg[0]=script name, arg[1..n]=args).
func Run(ctx context.Context, bus *gocan.Bus, path string, args ...string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return RunSource(ctx, bus, filepath.Base(path), string(src), args...)
}

// RunSource is Run for an in-memory script. name appears in Lua error
// messages and stack traces, and as arg[0].
func RunSource(ctx context.Context, bus *gocan.Bus, name, source string, args ...string) error {
	L := lua.NewState()
	defer L.Close()
	L.SetContext(ctx) // aborts the VM when ctx is cancelled

	registerFrameType(L)
	registerBitLib(L)
	setArgTable(L, name, args)

	sb := &scriptBus{ctx: ctx, bus: bus}
	ud := L.NewUserData()
	ud.Value = sb
	mt := L.NewTypeMetatable("canbus")
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"send":    busSend,
		"recv":    busRecv,
		"request": busRequest,
		"frames":  busFrames,
	}))
	L.SetMetatable(ud, mt)
	L.SetGlobal("bus", ud)

	L.SetGlobal("sleep", L.NewFunction(func(L *lua.LState) int {
		select {
		case <-time.After(time.Duration(L.CheckInt(1)) * time.Millisecond):
		case <-ctx.Done():
			L.RaiseError("sleep: %v", context.Cause(ctx))
		}
		return 0
	}))

	fn, err := L.Load(strings.NewReader(source), name)
	if err != nil {
		return err
	}
	L.Push(fn)
	return L.PCall(0, lua.MultRet, nil)
}

// setArgTable populates the standard Lua arg global: arg[0] is the script
// name, arg[1..n] the caller's args. Scripts read them as arg[1], arg[2], ...
func setArgTable(L *lua.LState, name string, args []string) {
	t := L.NewTable()
	t.RawSetInt(0, lua.LString(name))
	for i, a := range args {
		t.RawSetInt(i+1, lua.LString(a))
	}
	L.SetGlobal("arg", t)
}

type scriptBus struct {
	ctx context.Context
	bus *gocan.Bus
}

func checkBus(L *lua.LState) *scriptBus {
	sb, ok := L.CheckUserData(1).Value.(*scriptBus)
	if !ok {
		L.ArgError(1, "bus expected")
	}
	return sb
}

// tableToBytes converts a 1-based Lua byte table to a payload, rejecting
// anything classic CAN cannot carry.
func tableToBytes(L *lua.LState, t *lua.LTable) []byte {
	n := t.Len()
	if n > 8 {
		L.RaiseError("payload is %d bytes, classic CAN carries at most 8", n)
	}
	b := make([]byte, n)
	for i := range n {
		b[i] = byte(lua.LVAsNumber(t.RawGetInt(i + 1)))
	}
	return b
}

// restIDs collects the identifiers from stack position start onward.
func restIDs(L *lua.LState, start int) []uint32 {
	var ids []uint32
	for i := start; i <= L.GetTop(); i++ {
		ids = append(ids, uint32(L.CheckInt(i)))
	}
	return ids
}

// pushFrameResult maps a Recv/Request outcome to Lua: frame on success,
// nil+"timeout" on deadline, raised error when the bus is gone (the script
// cannot make progress, so it should die).
func pushFrameResult(L *lua.LState, f gocan.Frame, err error) int {
	switch {
	case err == nil:
		L.Push(newFrame(L, f))
		return 1
	case errors.Is(err, context.DeadlineExceeded):
		L.Push(lua.LNil)
		L.Push(lua.LString("timeout"))
		return 2
	default:
		L.RaiseError("%v", err)
		return 0
	}
}

func busSend(L *lua.LState) int {
	sb := checkBus(L)
	f := gocan.NewFrame(uint32(L.CheckInt(2)), tableToBytes(L, L.CheckTable(3)))
	if err := sb.bus.Send(sb.ctx, f); err != nil {
		L.RaiseError("send: %v", err)
	}
	return 0
}

func busRecv(L *lua.LState) int {
	sb := checkBus(L)
	timeout := time.Duration(L.CheckInt(2)) * time.Millisecond
	ctx, cancel := context.WithTimeout(sb.ctx, timeout)
	defer cancel()
	f, err := sb.bus.Recv(ctx, restIDs(L, 3)...)
	return pushFrameResult(L, f, err)
}

func busRequest(L *lua.LState) int {
	sb := checkBus(L)
	f := gocan.NewFrame(uint32(L.CheckInt(2)), tableToBytes(L, L.CheckTable(3)))
	timeout := time.Duration(L.CheckInt(4)) * time.Millisecond
	ctx, cancel := context.WithTimeout(sb.ctx, timeout)
	defer cancel()
	reply, err := sb.bus.Request(ctx, f, restIDs(L, 5)...)
	return pushFrameResult(L, reply, err)
}

// busFrames returns an iterator over matching frames. The subscription lives
// until the script's ctx ends.
// ponytail: breaking out of the loop early leaves the subscription draining
// until script end; add per-iterator cancel if scripts start looping over
// many short-lived subscriptions.
func busFrames(L *lua.LState) int {
	sb := checkBus(L)
	ch := sb.bus.Subscribe(sb.ctx, restIDs(L, 2)...)
	L.Push(L.NewFunction(func(L *lua.LState) int {
		f, ok := <-ch
		if !ok {
			if err := sb.bus.Err(); err != nil {
				L.RaiseError("%v", err)
			}
			L.Push(lua.LNil) // clean shutdown: end the loop
			return 1
		}
		L.Push(newFrame(L, f))
		return 1
	}))
	return 1
}

const frameTypeName = "canframe"

func registerFrameType(L *lua.LState) {
	mt := L.NewTypeMetatable(frameTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"id":    frameID,
		"len":   frameLen,
		"u8":    frameU8,
		"u16":   frameU16,
		"bytes": frameBytes,
		"hex":   frameHex,
	}))
	L.SetField(mt, "__tostring", L.NewFunction(frameString))
}

func newFrame(L *lua.LState, f gocan.Frame) *lua.LUserData {
	ud := L.NewUserData()
	ud.Value = f
	L.SetMetatable(ud, L.GetTypeMetatable(frameTypeName))
	return ud
}

func checkFrame(L *lua.LState) gocan.Frame {
	f, ok := L.CheckUserData(1).Value.(gocan.Frame)
	if !ok {
		L.ArgError(1, "frame expected")
	}
	return f
}

// checkOffset validates a 0-based byte offset against the frame length.
func checkOffset(L *lua.LState, f gocan.Frame, width int) int {
	off := L.CheckInt(2)
	if off < 0 || off+width > int(f.Length) {
		L.RaiseError("offset %d out of range for %d-byte frame", off, f.Length)
	}
	return off
}

func frameID(L *lua.LState) int {
	L.Push(lua.LNumber(checkFrame(L).ID))
	return 1
}

func frameLen(L *lua.LState) int {
	L.Push(lua.LNumber(checkFrame(L).Length))
	return 1
}

func frameU8(L *lua.LState) int {
	f := checkFrame(L)
	L.Push(lua.LNumber(f.Data[checkOffset(L, f, 1)]))
	return 1
}

func frameU16(L *lua.LState) int {
	f := checkFrame(L)
	off := checkOffset(L, f, 2)
	L.Push(lua.LNumber(binary.BigEndian.Uint16(f.Data[off:])))
	return 1
}

func frameBytes(L *lua.LState) int {
	f := checkFrame(L)
	t := L.NewTable()
	for i, b := range f.Bytes() {
		t.RawSetInt(i+1, lua.LNumber(b))
	}
	L.Push(t)
	return 1
}

func frameHex(L *lua.LState) int {
	f := checkFrame(L)
	L.Push(lua.LString(fmt.Sprintf("% X", f.Bytes())))
	return 1
}

func frameString(L *lua.LState) int {
	L.Push(lua.LString(checkFrame(L).String()))
	return 1
}
