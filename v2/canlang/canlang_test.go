package canlang

import (
	"context"
	"strings"
	"testing"

	gocan "github.com/roffe/gocan/v2"
)

// TestPingPong runs two scripts against one loopback bus: pong echoes every
// 0x100 frame back on 0x101, ping request/responds five times and checks the
// payloads.
func TestPingPong(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	bus, err := gocan.Open(ctx, "loopback", gocan.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	pongDone := make(chan error, 1)
	go func() {
		pongDone <- RunSource(ctx, bus, "pong", `
			for f in bus:frames(0x100) do
				bus:send(0x101, {0x02, f:u8(1)})
			end
		`)
	}()

	err = RunSource(ctx, bus, "ping", `
		sleep(20) -- let pong subscribe
		for i = 1, 5 do
			local f, err = bus:request(0x100, {0x01, i}, 1000, 0x101)
			assert(f, err)
			assert(f:id() == 0x101, "wrong id")
			assert(f:u8(0) == 0x02, "wrong marker")
			assert(f:u8(1) == i, "wrong seq")
		end
	`)
	if err != nil {
		t.Fatal(err)
	}

	cancel() // ends pong's frames loop
	if err := <-pongDone; err != nil && !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatal(err)
	}
}

// TestBitLib exercises the bit global from Lua, edge cases included.
func TestBitLib(t *testing.T) {
	bus, err := gocan.Open(t.Context(), "loopback", gocan.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	err = RunSource(t.Context(), bus, "bit", `
		assert(bit.band(0xF0, 0x3C) == 0x30)
		assert(bit.band(0xFF, 0x0F, 0x03) == 0x03)
		assert(bit.bor(0x01, 0x80) == 0x81)
		assert(bit.bxor(0xFF, 0x0F) == 0xF0)
		assert(bit.bnot(0) == 0xFFFFFFFF)
		assert(bit.band(-1, 0xFF) == 0xFF)      -- negatives wrap mod 2^32
		assert(bit.lshift(1, 4) == 16)
		assert(bit.lshift(1, 32) == 0)          -- shifted out
		assert(bit.rshift(0x80, 7) == 1)
		assert(bit.rshift(1, -4) == 16)         -- negative n shifts left
		assert(bit.extract(0x0A, 1) == 1)
		assert(bit.extract(0x0A, 2) == 0)
		assert(bit.extract(0xABCD, 4, 8) == 0xBC)
		assert(bit.extract(0xFFFFFFFF, 0, 32) == 0xFFFFFFFF)
		assert(bit.btest(0x08, 0x0C))
		assert(not bit.btest(0x01, 0x02))
		local ok = pcall(bit.extract, 1, 32)    -- out of range raises
		assert(not ok)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// TestRecvTimeout checks the nil,"timeout" convention.
func TestRecvTimeout(t *testing.T) {
	bus, err := gocan.Open(t.Context(), "loopback", gocan.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	err = RunSource(t.Context(), bus, "timeout", `
		local f, err = bus:recv(10, 0x7FF)
		assert(f == nil and err == "timeout", tostring(err))
	`)
	if err != nil {
		t.Fatal(err)
	}
}
