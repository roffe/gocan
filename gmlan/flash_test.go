package gmlan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

func testKey(seed []byte, level byte) (byte, byte) { return seed[0] ^ 0xA5, seed[1] ^ level }

// securityECU answers $27 with seed for an odd level, then accepts only the
// testKey for that seed and level.
func securityECU(seed ...byte) func(req []byte) [][]byte {
	var level byte
	return func(req []byte) [][]byte {
		switch {
		case req[0] != SECURITY_ACCESS || len(req) < 2:
			return nil
		case req[1]%2 == 1:
			level = req[1]
			return [][]byte{append([]byte{0x67, level}, seed...)}
		}
		hi, lo := testKey(seed, level)
		if req[1] != level+1 || !bytes.Equal(req[2:], []byte{hi, lo}) {
			return [][]byte{negative(SECURITY_ACCESS, 0x35)}
		}
		return [][]byte{{0x67, req[1]}}
	}
}

func TestRequestSecurityAccess(t *testing.T) {
	e := &fakeECU{handle: securityECU(0x12, 0x34)}
	cl, _ := newECUClient(t, e)
	var gotSeed []byte
	var gotLevel byte
	err := cl.RequestSecurityAccess(context.Background(), AccessLevelFD, 0, func(seed []byte, level byte) (byte, byte) {
		gotSeed, gotLevel = seed, level
		return testKey(seed, level)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotSeed, []byte{0x12, 0x34}) || gotLevel != AccessLevelFD {
		t.Errorf("seedfunc(% 02X, %02X), want seedfunc(12 34, FD)", gotSeed, gotLevel)
	}
	hi, lo := testKey([]byte{0x12, 0x34}, AccessLevelFD)
	e.check(t, []byte{SECURITY_ACCESS, 0xFD}, []byte{SECURITY_ACCESS, 0xFE, hi, lo})
}

// GMW3110: an ECU that is already unlocked answers the seed request with a
// zero seed and expects no key.
func TestRequestSecurityAccessAlreadyUnlocked(t *testing.T) {
	e := &fakeECU{handle: securityECU(0x00, 0x00)}
	cl, _ := newECUClient(t, e)
	err := cl.RequestSecurityAccess(context.Background(), AccessLevel01, 0, func([]byte, byte) (byte, byte) {
		t.Error("key computed for a zero seed")
		return 0, 0
	})
	if err != nil {
		t.Fatal(err)
	}
	e.check(t, []byte{SECURITY_ACCESS, AccessLevel01})
}

func TestRequestSecurityAccessRejected(t *testing.T) {
	t.Run("seed", func(t *testing.T) {
		const requiredTimeDelayNotExpired = 0x37
		e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{negative(SECURITY_ACCESS, requiredTimeDelayNotExpired)} }}
		cl, _ := newECUClient(t, e)
		err := cl.RequestSecurityAccess(context.Background(), AccessLevel01, 0, func([]byte, byte) (byte, byte) {
			t.Error("key computed without a seed")
			return 0, 0
		})
		wantNRC(t, err, SECURITY_ACCESS, requiredTimeDelayNotExpired)
		e.check(t, []byte{SECURITY_ACCESS, AccessLevel01})
	})
	t.Run("key", func(t *testing.T) {
		const invalidKey = 0x35
		e := &fakeECU{handle: securityECU(0x12, 0x34)}
		cl, _ := newECUClient(t, e)
		err := cl.RequestSecurityAccess(context.Background(), AccessLevel01, 0, func([]byte, byte) (byte, byte) { return 0xDE, 0xAD })
		wantNRC(t, err, SECURITY_ACCESS, invalidKey)
		e.check(t, []byte{SECURITY_ACCESS, AccessLevel01}, []byte{SECURITY_ACCESS, AccessLevel01 + 1, 0xDE, 0xAD})
	})
}

func TestSecurityAccessReplyForAnotherLevel(t *testing.T) {
	t.Run("seed", func(t *testing.T) {
		e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{{0x67, 0x03, 0x12, 0x34}} }}
		cl, _ := newECUClient(t, e)
		if seed, err := cl.SecurityAccessRequestSeed(context.Background(), AccessLevel01); err == nil {
			t.Fatalf("level 03 seed taken for level 01: % 02X", seed)
		}
	})
	t.Run("key", func(t *testing.T) {
		e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{{0x67, 0x04}} }}
		cl, _ := newECUClient(t, e)
		if err := cl.SecurityAccessSendKey(context.Background(), AccessLevel01, 0xAB, 0xCD); err == nil {
			t.Fatal("level 04 grant taken for level 02")
		}
	})
}

// The delay is sat out in pauses of at most a second, each followed by a
// tester present, before the key goes out.
func TestRequestSecurityAccessDelay(t *testing.T) {
	const s = time.Second
	for _, tc := range []struct {
		delay  time.Duration
		pauses []time.Duration
	}{
		{900 * time.Millisecond, []time.Duration{900 * time.Millisecond}},
		{s, []time.Duration{s}},
		{2500 * time.Millisecond, []time.Duration{s, s, 500 * time.Millisecond}},
	} {
		t.Run(tc.delay.String(), func(t *testing.T) {
			pauses := recordWaits(t)
			e := &fakeECU{handle: securityECU(0x12, 0x34)}
			cl, node := newECUClient(t, e)
			if err := cl.RequestSecurityAccess(context.Background(), AccessLevel01, tc.delay, testKey); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(*pauses, tc.pauses) {
				t.Errorf("paused %v, want %v", *pauses, tc.pauses)
			}
			var got []string
			for _, f := range node.Sent() {
				got = append(got, fmt.Sprintf("%03X % 02X", f.ID, f.Bytes()))
			}
			hi, lo := testKey([]byte{0x12, 0x34}, AccessLevel01)
			want := []string{"7E0 02 27 01"}
			for range tc.pauses {
				want = append(want, "101 FE 01 3E")
			}
			want = append(want, fmt.Sprintf("7E0 04 27 02 %02X %02X", hi, lo))
			if !slices.Equal(got, want) {
				t.Errorf("sent %q, want %q", got, want)
			}
		})
	}
}

func TestRequestSecurityAccessDelayEndsOnCancel(t *testing.T) {
	e := &fakeECU{handle: securityECU(0x12, 0x34)}
	cl, _ := newECUClient(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(20*time.Millisecond, cancel)
	start := time.Now()
	err := cl.RequestSecurityAccess(ctx, AccessLevel01, time.Second, testKey)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("returned %v after the start, want soon after the cancel at 20ms", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// A seed reply cut short must not be read as a seed: the zeroed bytes past
// its end make a zero seed ("already unlocked") or half a seed, whose wrong
// key burns one of the ECU's attempts.
func TestSecurityAccessShortSeedReply(t *testing.T) {
	short := func(t *testing.T, length uint8) *Client {
		node := (&fakeECU{handle: securityECU(0x12, 0x34)}).node()
		node.Fault = gocantest.Truncate(0, length)
		return newTestClient(t, node)
	}
	for _, length := range []uint8{3, 4} {
		t.Run(fmt.Sprint("SecurityAccessRequestSeed/", length, " bytes"), func(t *testing.T) {
			if seed, err := short(t, length).SecurityAccessRequestSeed(context.Background(), AccessLevel01); err == nil {
				t.Errorf("seed % 02X from a reply cut to %d bytes", seed, length)
			}
		})
	}
	t.Run("RequestSecurityAccess", func(t *testing.T) {
		if err := short(t, 3).RequestSecurityAccess(context.Background(), AccessLevel01, 0, testKey); err == nil {
			t.Error("unlocked by a seed reply cut to 3 bytes")
		}
	})
}

func TestReportProgrammedStateShortReply(t *testing.T) {
	node := (&fakeECU{handle: func([]byte) [][]byte { return [][]byte{{0xE2, 0x01}} }}).node()
	node.Fault = gocantest.Truncate(0, 2)
	if state, err := newTestClient(t, node).ReportProgrammedState(context.Background()); err == nil {
		t.Fatalf("state %02X from a reply cut to 2 bytes", state)
	}
}

func TestProgrammingMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		call func(*Client, context.Context) error
		req  []byte
	}{
		{"request", (*Client).ProgrammingModeRequest, []byte{PROGRAMMING_MODE, 0x01}},
		{"request high speed", (*Client).ProgrammingModeRequestHighSpeed, []byte{PROGRAMMING_MODE, 0x02}},
		// enableProgrammingMode has no response.
		{"enable", (*Client).ProgrammingModeEnable, []byte{PROGRAMMING_MODE, 0x03}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeECU{handle: func(req []byte) [][]byte {
				if req[1] == 0x03 {
					return nil
				}
				return [][]byte{{0xE5}}
			}}
			cl, _ := newECUClient(t, e)
			if err := tc.call(cl, context.Background()); err != nil {
				t.Fatal(err)
			}
			e.check(t, tc.req)
		})
	}
	t.Run("unknown level", func(t *testing.T) {
		e := &fakeECU{handle: silentECU}
		cl, _ := newECUClient(t, e)
		if err := cl.ProgrammingMode(context.Background(), 0x04); err == nil {
			t.Fatal("level 04 accepted")
		}
		e.check(t)
	})
	t.Run("silent", func(t *testing.T) {
		cl, _ := newECUClient(t, &fakeECU{handle: silentECU})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := cl.ProgrammingModeRequest(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want a timeout", err)
		}
	})
}

func TestProgrammingModeNegativeResponse(t *testing.T) {
	e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{negative(PROGRAMMING_MODE, 0x22)} }}
	cl, _ := newECUClient(t, e)
	wantNRC(t, cl.ProgrammingModeRequest(context.Background()), PROGRAMMING_MODE, 0x22)
}

// Starting the loader and entering programming mode take the ECU longer
// than an ordinary request, and the client waits for them longer.
func TestSlowAnswers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		call func(context.Context, *Client) error
		resp []byte
	}{
		{"Execute", func(ctx context.Context, cl *Client) error { return cl.Execute(ctx, 0x102400) }, []byte{0x76}},
		{"ProgrammingModeRequest", func(ctx context.Context, cl *Client) error { return cl.ProgrammingModeRequest(ctx) }, []byte{0xE5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node := (&fakeECU{handle: func([]byte) [][]byte { return [][]byte{tc.resp} }}).node()
			node.Delay = 100 * time.Millisecond
			if err := tc.call(context.Background(), newTestClient(t, node)); err != nil {
				t.Fatalf("answer after twice the default timeout: %v", err)
			}
		})
	}
}

func TestTransferData(t *testing.T) {
	e := &fakeECU{}
	cl, node := newECUClient(t, e)
	if err := cl.TransferData(context.Background(), 0x00, 0xF0, 0x102400); err != nil {
		t.Fatal(err)
	}
	want := []byte{0x10, 0xF0, TRANSFER_DATA, 0x00, 0x00, 0x10, 0x24, 0x00}
	if sent := node.Sent(); len(sent) != 1 || !bytes.Equal(sent[0].Bytes(), want) {
		t.Errorf("sent %v, want first frame [% 02X]", sent, want)
	}
	e.check(t)
}

// The caller streams the consecutive frames after TransferData without
// waiting, so anything but clear-to-send for the whole block is a failure.
func TestTransferDataRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		reply []byte
	}{
		{"block size 1", []byte{0x30, 0x01, 0x00, 0, 0, 0, 0, 0}},
		{"overflow", []byte{0x32, 0x00, 0x00, 0, 0, 0, 0, 0}},
		{"silent", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &gocantest.Node{Reply: func(gocan.Frame) []gocan.Frame {
				if tc.reply == nil {
					return nil
				}
				return []gocan.Frame{gocan.NewFrame(0x7E8, tc.reply)}
			}}
			if err := newTestClient(t, node).TransferData(context.Background(), 0x00, 0xF0, 0x102400); err == nil {
				t.Fatal("transfer started")
			}
		})
	}
	t.Run("negative response", func(t *testing.T) {
		node := &gocantest.Node{Reply: func(gocan.Frame) []gocan.Frame { return []gocan.Frame{sf(negative(TRANSFER_DATA, 0x22)...)} }}
		wantNRC(t, newTestClient(t, node).TransferData(context.Background(), 0x00, 0xF0, 0x102400), TRANSFER_DATA, 0x22)
	})
}

func counting(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

// 120 data bytes need 17 consecutive frames after the first frame's 4: the
// sequence number runs 21..2F, wraps to 20 and goes on to 21.
func TestWriteDataByIdentifierSequenceWraps(t *testing.T) {
	data := counting(120)
	e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
	cl, _ := newECUClient(t, e)
	if err := cl.WriteDataByIdentifier(context.Background(), 0x90, data); err != nil {
		t.Fatal(err)
	}
	e.check(t, append([]byte{WRITE_DATA_BY_IDENTIFIER, 0x90}, data...))
}

func TestWriteDataByIdentifierHonoursSTmin(t *testing.T) {
	const stmin = 20 // ms
	e := &fakeECU{stmin: stmin, handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
	cl, _ := newECUClient(t, e)
	start := time.Now()
	if err := cl.WriteDataByIdentifier(context.Background(), 0x90, vin); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < stmin*time.Millisecond {
		t.Errorf("two consecutive frames sent %v apart, want at least STmin %d ms", elapsed, stmin)
	}
	e.check(t, vinWrite)
}

func TestWriteDataByIdentifierSixBytes(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6}
	e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
	cl, _ := newECUClient(t, e)
	if err := cl.WriteDataByIdentifier(context.Background(), 0x01, data); err != nil {
		t.Error(err)
	}
	e.check(t, append([]byte{WRITE_DATA_BY_IDENTIFIER, 0x01}, data...))
}

// slowECU is e answering flow control 5 ms after the frame that asks for it
// and requests 20 ms after their last frame. A consecutive frame sent before
// the flow control it waits for fails t.
func slowECU(t *testing.T, e *fakeECU) *Client {
	t.Helper()
	var (
		node *gocantest.Node
		mu   sync.Mutex
		owed bool
	)
	node = &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
		mu.Lock()
		if f.Data[0]>>4 == 2 && owed {
			t.Errorf("consecutive frame [% 02X] sent before the flow control", f.Bytes())
		}
		mu.Unlock()
		for _, r := range e.reply(f) {
			fc := r.Data[0]>>4 == 3
			d := 20 * time.Millisecond
			if fc {
				d = 5 * time.Millisecond
				mu.Lock()
				owed = true
				mu.Unlock()
			}
			time.AfterFunc(d, func() {
				if fc {
					mu.Lock()
					owed = false
					mu.Unlock()
				}
				node.Deliver(r)
			})
		}
		return nil
	}}
	return newTestClient(t, node)
}

// With a block size the tester waits for flow control after every block of
// consecutive frames. A tester that does not finds that flow control in the
// window for the write's answer.
func TestTransferWaitsForFlowControl(t *testing.T) {
	t.Parallel()
	data := counting(40) // 4 bytes in the first frame, 6 consecutive frames
	for _, bs := range []byte{1, 2, 4} {
		t.Run(fmt.Sprint("block size ", bs), func(t *testing.T) {
			t.Parallel()
			e := &fakeECU{bs: bs, handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
			if err := slowECU(t, e).WriteDataByIdentifier(context.Background(), 0x90, data); err != nil {
				t.Fatal(err)
			}
			e.check(t, append([]byte{WRITE_DATA_BY_IDENTIFIER, 0x90}, data...))
		})
	}
	t.Run("refused", func(t *testing.T) {
		t.Parallel()
		e := &fakeECU{bs: 1, handle: func([]byte) [][]byte { return [][]byte{negative(WRITE_DATA_BY_IDENTIFIER, 0x22)} }}
		wantNRC(t, slowECU(t, e).WriteDataByIdentifier(context.Background(), 0x90, vin), WRITE_DATA_BY_IDENTIFIER, 0x22)
	})
}

func TestTransferFlowControl(t *testing.T) {
	waitFC := gocan.NewFrame(0x7E8, []byte{0x31, 0x00, 0x00, 0, 0, 0, 0, 0})
	for _, tc := range []struct {
		name   string
		before []gocan.Frame // sent ahead of the ECU's clear to send
		ok     bool
	}{
		{"wait", slices.Repeat([]gocan.Frame{waitFC}, maxFlowControlWaits), true},
		{"too many waits", slices.Repeat([]gocan.Frame{waitFC}, maxFlowControlWaits+1), false},
		{"overflow", []gocan.Frame{gocan.NewFrame(0x7E8, []byte{0x32, 0x00, 0x00, 0, 0, 0, 0, 0})}, false},
		{"short", []gocan.Frame{gocan.NewFrame(0x7E8, []byte{0x30, 0x00})}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
			node := &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
				out := e.reply(f)
				if f.Data[0]>>4 == 1 {
					out = append(slices.Clone(tc.before), out...)
				}
				return out
			}}
			err := newTestClient(t, node).WriteDataByIdentifier(context.Background(), 0x90, vin)
			if tc.ok {
				if err != nil {
					t.Fatal(err)
				}
				e.check(t, vinWrite)
				return
			}
			if err == nil {
				t.Fatal("transfer went on")
			}
			if sent := node.Sent(); len(sent) != 1 {
				t.Errorf("sent %d consecutive frames, want none", len(sent)-1)
			}
		})
	}
}

func TestSeparationTime(t *testing.T) {
	for b, want := range map[byte]time.Duration{
		0x00: 0,
		0x14: 20 * time.Millisecond,
		0x7F: 127 * time.Millisecond,
		0xF1: 100 * time.Microsecond,
		0xF9: 900 * time.Microsecond,
		0x80: 127 * time.Millisecond, // reserved
		0xF0: 127 * time.Millisecond, // reserved
		0xFA: 127 * time.Millisecond, // reserved
	} {
		if got := separationTime(b); got != want {
			t.Errorf("separationTime(%#02x) = %v, want %v", b, got, want)
		}
	}
}

// STmin separates consecutive frames, not a flow control from the first
// consecutive frame after it.
func TestTransferPausesSTminBetweenConsecutiveFrames(t *testing.T) {
	pauses := recordWaits(t)
	data := counting(40) // 6 consecutive frames
	e := &fakeECU{stmin: 0xF5, handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
	cl, _ := newECUClient(t, e)
	if err := cl.WriteDataByIdentifier(context.Background(), 0x90, data); err != nil {
		t.Fatal(err)
	}
	if want := slices.Repeat([]time.Duration{500 * time.Microsecond}, 5); !slices.Equal(*pauses, want) {
		t.Errorf("paused %v, want %v", *pauses, want)
	}
	e.check(t, append([]byte{WRITE_DATA_BY_IDENTIFIER, 0x90}, data...))
}

func TestTransferStopsOnSendError(t *testing.T) {
	pauses := recordWaits(t)
	unplugged := errors.New("adapter unplugged")
	node := &gocantest.Node{}
	node.Reply = func(f gocan.Frame) []gocan.Frame {
		if f.Data[0]>>4 == 1 {
			node.Deliver(gocan.NewFrame(0x7E8, []byte{0x30, 0x00, 0x7F, 0, 0, 0, 0, 0}))
			node.Unplug(unplugged)
		}
		return nil
	}
	if err := newTestClient(t, node).WriteDataByIdentifier(context.Background(), 0x90, counting(40)); !errors.Is(err, unplugged) {
		t.Fatalf("err = %v, want %v", err, unplugged)
	}
	if len(*pauses) != 0 {
		t.Errorf("paused %v after the failed send", *pauses)
	}
}

// A first frame carries an 8-bit length.
func TestRequestTooLongForOneTransfer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(context.Context, *Client) error
		ok    bool
	}{
		{"WriteDataByIdentifier 253", func(ctx context.Context, cl *Client) error { return cl.WriteDataByIdentifier(ctx, 0x90, counting(253)) }, true},
		{"WriteDataByIdentifier 254", func(ctx context.Context, cl *Client) error { return cl.WriteDataByIdentifier(ctx, 0x90, counting(254)) }, false},
		{"WriteDataByAddress 249", func(ctx context.Context, cl *Client) error {
			return cl.WriteDataByAddress(ctx, 0x102030, counting(249))
		}, true},
		{"WriteDataByAddress 250", func(ctx context.Context, cl *Client) error {
			return cl.WriteDataByAddress(ctx, 0x102030, counting(250))
		}, false},
		{"DynamicallyDefineMessage 127", func(ctx context.Context, cl *Client) error {
			return cl.DynamicallyDefineMessage(ctx, make([]uint16, 127)...)
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{{req[0] + 0x40, req[1]}} }}
			cl, node := newECUClient(t, e)
			err := tc.write(context.Background(), cl)
			switch {
			case tc.ok && err != nil:
				t.Fatal(err)
			case tc.ok:
				e.noViolations(t)
			case err == nil:
				t.Fatal("too long a request accepted")
			case len(node.Sent()) != 0:
				t.Errorf("sent %v", node.Sent())
			}
		})
	}
}

func TestMultiFrameWriteRefusedAtFirstFrame(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(context.Context, *Client) error
	}{
		{"WriteDataByIdentifier", func(ctx context.Context, cl *Client) error { return cl.WriteDataByIdentifier(ctx, 0x90, vin) }},
		{"WriteDataByAddress", func(ctx context.Context, cl *Client) error { return cl.WriteDataByAddress(ctx, 0x102030, ramData) }},
		{"WriteDataByAddress2", func(ctx context.Context, cl *Client) error { return cl.WriteDataByAddress2(ctx, 0x102030, ramData) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &gocantest.Node{Reply: func(gocan.Frame) []gocan.Frame {
				return []gocan.Frame{sf(negative(WRITE_DATA_BY_IDENTIFIER, 0x22)...)}
			}}
			wantNRC(t, tc.write(context.Background(), newTestClient(t, node)), WRITE_DATA_BY_IDENTIFIER, 0x22)
			if sent := node.Sent(); len(sent) != 1 {
				t.Errorf("sent %d frames after the refusal, want none", len(sent)-1)
			}
		})
	}
}

var addressWrites = []struct {
	name  string
	write func(*Client, context.Context, uint32, []byte) error
}{
	{"WriteDataByAddress", (*Client).WriteDataByAddress},
	{"WriteDataByAddress2", (*Client).WriteDataByAddress2},
}

func TestWriteDataByAddressSequenceWraps(t *testing.T) {
	data := counting(120)
	for _, w := range addressWrites {
		t.Run(w.name, func(t *testing.T) {
			e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
			cl, node := newECUClient(t, e)
			if err := w.write(cl, context.Background(), 0x102030, data); err != nil {
				t.Fatal(err)
			}
			first := []byte{0x10, 6 + 120, WRITE_DATA_BY_IDENTIFIER, 0x15, 0x10, 0x20, 0x30, 120}
			if sent := node.Sent(); len(sent) == 0 || !bytes.Equal(sent[0].Bytes(), first) {
				t.Errorf("first frame %v, want [% 02X]", sent[:min(1, len(sent))], first)
			}
			e.check(t, append(slices.Clone(first[2:]), data...))
		})
	}
}

// Busy after a multi-frame request asks for the whole request again,
// whatever data it carries.
func TestWriteDataByAddressBusy(t *testing.T) {
	t.Parallel()
	for _, w := range addressWrites {
		for _, first := range []byte{0x01, WRITE_DATA_BY_IDENTIFIER} {
			t.Run(fmt.Sprintf("%s/data %02X", w.name, first), func(t *testing.T) {
				t.Parallel()
				data := []byte{first, 1, 2, 3, 4, 5, 6}
				e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{negative(req[0], 0x21)} }}
				cl, _ := newECUClient(t, e)
				wantNRC(t, w.write(cl, context.Background(), 0x102030, data), WRITE_DATA_BY_IDENTIFIER, 0x21)
				req := append([]byte{WRITE_DATA_BY_IDENTIFIER, 0x15, 0x10, 0x20, 0x30, byte(len(data))}, data...)
				e.check(t, slices.Repeat([][]byte{req}, 1+busyRetries)...)
			})
		}
	}
	t.Run("then done", func(t *testing.T) {
		t.Parallel()
		replies := [][]byte{negative(WRITE_DATA_BY_IDENTIFIER, 0x21), {0x7B, 0x15}}
		e := &fakeECU{handle: func([]byte) [][]byte {
			r := replies[0]
			replies = replies[1:]
			return [][]byte{r}
		}}
		cl, _ := newECUClient(t, e)
		if err := cl.WriteDataByAddress(context.Background(), 0x102030, ramData); err != nil {
			t.Fatal(err)
		}
		e.check(t, ramWrite, ramWrite)
	})
}

// The apptool write goes out as a first frame even when it would fit a
// single frame, and refuses to write nothing.
func TestWriteDataByAddressShort(t *testing.T) {
	t.Run("one byte", func(t *testing.T) {
		e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
		cl, node := newECUClient(t, e)
		if err := cl.WriteDataByAddress(context.Background(), 0x102030, []byte{0xAA}); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, f := range node.Sent() {
			got = append(got, fmt.Sprintf("% 02X", f.Bytes()))
		}
		if want := []string{"10 07 3B 15 10 20 30 01", "21 AA"}; !slices.Equal(got, want) {
			t.Errorf("sent %q, want %q", got, want)
		}
		e.check(t, []byte{WRITE_DATA_BY_IDENTIFIER, 0x15, 0x10, 0x20, 0x30, 0x01, 0xAA})
	})
	t.Run("no data", func(t *testing.T) {
		cl, node := newECUClient(t, &fakeECU{handle: silentECU})
		if err := cl.WriteDataByAddress(context.Background(), 0x102030, nil); err == nil {
			t.Fatal("empty write accepted")
		}
		if sent := node.Sent(); len(sent) != 0 {
			t.Errorf("sent %v", sent)
		}
	})
}

func TestExecuteDoesNotLog(t *testing.T) {
	var out bytes.Buffer
	defer log.SetOutput(log.Writer())
	log.SetOutput(&out)
	e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{{0x76}} }}
	cl, _ := newECUClient(t, e)
	if err := cl.Execute(context.Background(), 0x102400); err != nil {
		t.Fatal(err)
	}
	if out.Len() > 0 {
		t.Errorf("logged %q", out.String())
	}
}

// Every reply frame of a multi-frame write is dropped, duplicated and
// truncated in turn: the write reports success only when the ECU received
// the whole request.
func TestWritesUnderFaults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		frames int // reply frames in a clean write
		write  func(context.Context, *Client) error
		want   []byte
	}{
		{"WriteDataByIdentifier", 2, func(ctx context.Context, cl *Client) error { return cl.WriteDataByIdentifier(ctx, 0x90, vin) }, vinWrite},
		{"WriteDataByAddress", 4, func(ctx context.Context, cl *Client) error { return cl.WriteDataByAddress(ctx, 0x102030, ramData) }, ramWrite},
		{"WriteDataByAddress2", 4, func(ctx context.Context, cl *Client) error { return cl.WriteDataByAddress2(ctx, 0x102030, ramData) }, ramWrite},
	} {
		run := func(name string, fault func(int, gocan.Frame) []gocan.Frame, clean bool) {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{{0x7B, req[1]}} }}
				node := e.node()
				node.Fault = fault
				err := tc.write(context.Background(), newTestClient(t, node))
				e.noViolations(t)
				reqs := e.requests()
				received := len(reqs) == 1 && bytes.Equal(reqs[0], tc.want)
				switch {
				case clean && (err != nil || !received):
					t.Fatalf("clean write: %v, ECU received % 02X", err, reqs)
				case err == nil && !received:
					t.Fatalf("write reported done, ECU received % 02X", reqs)
				}
			})
		}
		run(tc.name+"/clean", nil, true)
		for k := range tc.frames {
			for fname, fault := range faults(k) {
				run(fmt.Sprintf("%s/%s %d", tc.name, fname, k), fault, false)
			}
		}
	}
}
