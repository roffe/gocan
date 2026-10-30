package t7kwp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

var stockPair = SeedKey{XOR: 0x8142, Sub: 0x2356}

func TestStartSession(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start func(*Client, context.Context) error
		reqID []byte
	}{
		{"StartSession", func(c *Client, ctx context.Context) error { return c.StartSession(ctx, INIT_MSG_ID, INIT_RESP_ID) }, []byte{0x02, 0x40}},
		{"StartSession2", func(c *Client, ctx context.Context) error { return c.StartSession2(ctx, INIT_MSG_ID, INIT_RESP_ID) }, []byte{0x07, 0x40}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{closed: true}
			node := e.node()
			c := New(gocantest.Open(t, node))
			if err := tc.start(c, t.Context()); err != nil {
				t.Fatal(err)
			}
			want := append([]byte{0x3F, START_COM_REQ, 0x00, 0x11}, tc.reqID[0], tc.reqID[1], 0, 0)
			if got := sentOn(node, INIT_MSG_ID); len(got) != 1 || !bytes.Equal(got[0], want) {
				t.Fatalf("sent %02X on 0x220, want [% 02X]", got, want)
			}
			if c.responseID != 0x258 {
				t.Fatalf("response id 0x%03X, want 0x258 from the reply", c.responseID)
			}
			if err := c.TesterPresent(t.Context()); err != nil {
				t.Fatalf("TesterPresent on the new session: %v", err)
			}
		})
	}
}

// A T7 that already holds a session ignores startCommunication.
func TestStartSessionIgnored(t *testing.T) {
	shortTimeout(t)
	c, _ := newT7(t, &fakeT7{})
	if err := c.StartSession(t.Context(), INIT_MSG_ID, INIT_RESP_ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a timeout", err)
	}
}

func TestStartSessionRejected(t *testing.T) {
	c := New(gocantest.Open(t, &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
		return []gocan.Frame{gocan.NewFrame(INIT_RESP_ID, []byte{0x40, 0xBF, 0x21, 0x7F, 0x81, 0x10, 0x00, 0x00})}
	}}))
	if err := c.StartSession(t.Context(), INIT_MSG_ID, INIT_RESP_ID); !errors.Is(err, ErrGeneralReject) {
		t.Fatalf("err = %v, want %v", err, ErrGeneralReject)
	}
}

func TestStartSessionShortReply(t *testing.T) {
	e := &fakeT7{closed: true}
	node := e.node()
	node.Fault = gocantest.Truncate(0, 6)
	c := New(gocantest.Open(t, node))
	if err := c.StartSession(t.Context(), INIT_MSG_ID, INIT_RESP_ID); err == nil {
		t.Fatalf("StartSession accepted a 6-byte reply, response id now 0x%03X", c.responseID)
	}
}

func TestStopSession(t *testing.T) {
	e := &fakeT7{}
	c, node := newT7(t, e)
	if err := c.StopSession(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, want := sentOn(node, REQ_MSG_ID), []byte{0x40, 0xA1, 0x01, STOP_COM_REQ, 0, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("sent %02X, want [% 02X]", got, want)
	}
	if got := acks(node); !bytes.Equal(got, []byte{0x80}) {
		t.Fatalf("acks % 02X, want 80", got)
	}
	if !e.closed {
		t.Fatal("ECU still holds the session")
	}
}

// A reply to some other service does not end the session.
func TestStopSessionOtherService(t *testing.T) {
	c, _ := newT7(t, &fakeT7{answer: map[byte]func([]byte) []byte{STOP_COM_REQ: answerWith(TESTER_PRESENT | 0x40)}})
	if err := c.StopSession(t.Context()); err == nil {
		t.Fatal("StopSession accepted a 7E reply")
	}
}

// With no session the ECU ignores stopCommunication.
func TestStopSessionWithoutSession(t *testing.T) {
	shortTimeout(t)
	c, _ := newT7(t, &fakeT7{closed: true})
	if err := c.StopSession(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a timeout", err)
	}
}

func TestResetECU(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  func([]byte) []byte
		wantErr error
	}{
		{name: "positive"},
		{name: "refused", answer: answerWith(0x7F, ECU_RESET, CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR), wantErr: ErrConditionsNotCorrectOrRequestSequenceError},
		{name: "not power-on reset", answer: answerWith(ECU_RESET|0x40, 0x00), wantErr: errAny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{}
			if tc.answer != nil {
				e.answer = map[byte]func([]byte) []byte{ECU_RESET: tc.answer}
			}
			c, node := newT7(t, e)
			err := c.ResetECU(t.Context())
			checkErrIs(t, err, tc.wantErr)
			if got, want := sentOn(node, REQ_MSG_ID), []byte{0x40, 0xA1, 0x02, ECU_RESET, 0x01, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
				t.Fatalf("sent %02X, want [% 02X]", got, want)
			}
		})
	}
}

// errAny in a table means "some error"; checkErrIs accepts any non-nil error for it.
var errAny = errors.New("any error")

func checkErrIs(t *testing.T, err, want error) {
	t.Helper()
	switch {
	case want == nil && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want == errAny && err == nil, want != nil && want != errAny && !errors.Is(err, want):
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// A new session starts with no security access, so the cached grant must not
// outlive the session that earned it.
func TestNewSessionDropsSecurityAccess(t *testing.T) {
	e := &fakeT7{closed: true, pair: stockPair, seed: 0x1234}
	c := New(gocantest.Open(t, e.node()))
	ctx := t.Context()
	if err := c.StartSession(ctx, INIT_MSG_ID, INIT_RESP_ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.RequestSecurityAccess(ctx, false); !ok || err != nil {
		t.Fatalf("first session: %v, %v", ok, err)
	}
	if err := c.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.StartSession(ctx, INIT_MSG_ID, INIT_RESP_ID); err != nil {
		t.Fatal(err)
	}
	before := e.count(SECURITY_ACCESS)
	if ok, err := c.RequestSecurityAccess(ctx, false); !ok || err != nil {
		t.Fatalf("second session: %v, %v", ok, err)
	}
	if e.count(SECURITY_ACCESS) == before || e.access != DEVELOPMENT_PRIORITY {
		t.Fatal("second session reported unlocked without a seed/key exchange")
	}
}

func TestSecurityAccess(t *testing.T) {
	e := &fakeT7{pair: stockPair, seed: 0x1234}
	c, node := newT7(t, e)
	ok, err := c.SecurityAccess(t.Context(), DEVELOPMENT_PRIORITY, stockPair)
	if !ok || err != nil {
		t.Fatalf("SecurityAccess = %v, %v", ok, err)
	}
	// key = ((0x1234<<2) ^ 0x8142) - 0x2356 = 0xA63C
	want := [][]byte{
		{0x40, 0xA1, 0x02, SECURITY_ACCESS, 0x05, 0, 0, 0},
		{0x40, 0xA1, 0x04, SECURITY_ACCESS, 0x06, 0xA6, 0x3C, 0},
	}
	if got := sentOn(node, REQ_MSG_ID); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatalf("sent %02X, want %02X", got, want)
	}
	if got := acks(node); !bytes.Equal(got, []byte{0x80, 0x80}) {
		t.Fatalf("acks % 02X, want 80 80", got)
	}
	if e.access != DEVELOPMENT_PRIORITY || !c.gotSequrityAccess {
		t.Fatal("access not granted")
	}
}

// A T7 answers a wrong key with securityAccessDenied.
func TestSecurityAccessWrongKey(t *testing.T) {
	c, _ := newT7(t, &fakeT7{pair: SeedKey{XOR: 0x4081, Sub: 0x1F6F}, seed: 0x1234})
	ok, err := c.SecurityAccess(t.Context(), DEVELOPMENT_PRIORITY, stockPair)
	if ok || !errors.Is(err, ErrSecurityAccessDeniedOrRequested) {
		t.Fatalf("SecurityAccess = %v, %v; want false, %v", ok, err, ErrSecurityAccessDeniedOrRequested)
	}
	if c.gotSequrityAccess {
		t.Fatal("access recorded after a rejected key")
	}
}

// After too many wrong keys the seed request itself is refused, and no key
// goes out.
func TestSecurityAccessLockedOut(t *testing.T) {
	e := &fakeT7{pair: stockPair, locked: true}
	c, _ := newT7(t, e)
	ok, err := c.SecurityAccess(t.Context(), DEVELOPMENT_PRIORITY, stockPair)
	if ok || !errors.Is(err, ErrRequiredTimeDelayNotExpired) {
		t.Fatalf("SecurityAccess = %v, %v; want false, %v", ok, err, ErrRequiredTimeDelayNotExpired)
	}
	if n := len(e.requests); n != 1 {
		t.Fatalf("%d requests, want only the seed request", n)
	}
}

// An unlocked ECU hands out seed 0000: access is already granted and no key
// is sent.
func TestSecurityAccessZeroSeed(t *testing.T) {
	if os.Getenv("GOCAN_BUG_TESTS") == "" {
		t.Skip("BUG: seed 0000 (already unlocked) is answered with a key, which the ECU rejects")
	}
	e := &fakeT7{pair: stockPair, access: DEVELOPMENT_PRIORITY, key: 0x1111}
	c, node := newT7(t, e)
	ok, err := c.SecurityAccess(t.Context(), DEVELOPMENT_PRIORITY, stockPair)
	if !ok || err != nil {
		t.Fatalf("SecurityAccess = %v, %v; want true, nil", ok, err)
	}
	if got := sentOn(node, REQ_MSG_ID); len(got) != 1 {
		t.Fatalf("sent %02X, want only the seed request", got)
	}
}

func TestRequestSeed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		answer   func([]byte) []byte
		wantSeed uint16
		wantErr  error
	}{
		{name: "positive", wantSeed: 0xBEEF},
		{name: "negative", answer: answerWith(0x7F, SECURITY_ACCESS, SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT), wantErr: ErrSubFunctionNotSupportedOrInvalidFormat},
		{name: "other service", answer: answerWith(TESTER_PRESENT | 0x40), wantErr: errAny},
		{name: "no seed bytes", answer: answerWith(SECURITY_ACCESS|0x40, HIGH_PRIORITY), wantErr: errAny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{l3Seed: 0xBEEF}
			if tc.answer != nil {
				e.answer = map[byte]func([]byte) []byte{SECURITY_ACCESS: tc.answer}
			}
			c, node := newT7(t, e)
			seed, err := c.RequestSeed(t.Context(), HIGH_PRIORITY)
			checkErrIs(t, err, tc.wantErr)
			if seed != tc.wantSeed {
				t.Fatalf("seed %04X, want %04X", seed, tc.wantSeed)
			}
			if got, want := sentOn(node, REQ_MSG_ID), []byte{0x40, 0xA1, 0x02, SECURITY_ACCESS, HIGH_PRIORITY, 0, 0, 0}; len(got) != 1 || !bytes.Equal(got[0], want) {
				t.Fatalf("sent %02X, want [% 02X]", got, want)
			}
			if got := acks(node); !bytes.Equal(got, []byte{0x80}) {
				t.Fatalf("acks % 02X, want 80", got)
			}
		})
	}
}

func TestRequestSeedShortReply(t *testing.T) {
	e := &fakeT7{l3Seed: 0xBEEF}
	node := e.node()
	node.Fault = gocantest.Truncate(0, 5)
	seed, err := newClient(t, node).RequestSeed(t.Context(), HIGH_PRIORITY)
	if err == nil {
		t.Fatalf("RequestSeed = %04X, nil; want an error", seed)
	}
}

func TestRequestSecurityAccess(t *testing.T) {
	t.Run("stock pair", func(t *testing.T) {
		e := &fakeT7{pair: stockPair, seed: 0x1234}
		c, _ := newT7(t, e)
		if ok, err := c.RequestSecurityAccess(t.Context(), false); !ok || err != nil {
			t.Fatalf("RequestSecurityAccess = %v, %v", ok, err)
		}
		if got := e.count(SECURITY_ACCESS); got != 2 {
			t.Fatalf("%d securityAccess requests, want seed + key", got)
		}

		if ok, err := c.RequestSecurityAccess(t.Context(), false); !ok || err != nil {
			t.Fatalf("cached: %v, %v", ok, err)
		}
		if got := e.count(SECURITY_ACCESS); got != 2 {
			t.Fatalf("cached grant sent %d more requests", got-2)
		}
	})

	t.Run("custom pair first", func(t *testing.T) {
		custom := SeedKey{XOR: 0x1234, Sub: 0x5678}
		e := &fakeT7{pair: custom, seed: 0x1234}
		c, node := newT7(t, e)
		c.SetSeedKey(&custom)
		if ok, err := c.RequestSecurityAccess(t.Context(), false); !ok || err != nil {
			t.Fatalf("RequestSecurityAccess = %v, %v", ok, err)
		}
		// key = ((0x1234<<2) ^ 0x1234) - 0x5678 = 0x046C
		want := []byte{0x40, 0xA1, 0x04, SECURITY_ACCESS, 0x06, 0x04, 0x6C, 0}
		if got := sentOn(node, REQ_MSG_ID); len(got) != 2 || !bytes.Equal(got[1], want) {
			t.Fatalf("sent %02X, want the custom key [% 02X] second", got, want)
		}
		c.SetSeedKey(nil)
		if c.seedKey != nil {
			t.Fatal("SetSeedKey(nil) kept the custom pair")
		}
	})
}

// A rejected custom pair falls back to the stock pairs. The client waits a
// fixed 3 s after each failed exchange.
func TestRequestSecurityAccessFallsBackToStock(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the client's 3 s retry delay")
	}
	e := &fakeT7{pair: stockPair, seed: 0x1234}
	c, _ := newT7(t, e)
	c.SetSeedKey(&SeedKey{XOR: 0x1111, Sub: 0x2222})
	if ok, err := c.RequestSecurityAccess(t.Context(), false); !ok || err != nil {
		t.Fatalf("RequestSecurityAccess = %v, %v", ok, err)
	}
	if got := e.count(SECURITY_ACCESS); got != 4 {
		t.Fatalf("%d securityAccess requests, want two exchanges", got)
	}
}

func TestRequestSecurityAccessKeepsNRC(t *testing.T) {
	if os.Getenv("GOCAN_BUG_TESTS") == "" {
		t.Skip("BUG: RequestSecurityAccess drops the ECU's NRC (lockout, timeout) and returns a bare \"security access denied\"")
	}
	c, _ := newT7(t, &fakeT7{pair: stockPair, locked: true})
	if ok, err := c.RequestSecurityAccess(t.Context(), false); ok || !errors.Is(err, ErrRequiredTimeDelayNotExpired) {
		t.Fatalf("RequestSecurityAccess = %v, %v; want false, %v", ok, err, ErrRequiredTimeDelayNotExpired)
	}
}

func TestRequestSecurityAccessHonoursCancel(t *testing.T) {
	if os.Getenv("GOCAN_BUG_TESTS") == "" {
		t.Skip("BUG: RequestSecurityAccess sleeps 3 s per pair with time.Sleep and ignores a cancelled context")
	}
	c, _ := newT7(t, &fakeT7{closed: true})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	_, err := c.RequestSecurityAccess(ctx, false)
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("err = %v after %v, want context.Canceled at once", err, time.Since(start))
	}
}

// Security access does not outlive the session that earned it.
func TestSessionEndDropsSecurityAccess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		closed bool
		end    func(*Client, context.Context) error
	}{
		{"StopSession", false, (*Client).StopSession},
		{"ResetECU", false, (*Client).ResetECU},
		{"StartSession", true, func(c *Client, ctx context.Context) error { return c.StartSession(ctx, INIT_MSG_ID, INIT_RESP_ID) }},
		{"StartSession2", true, func(c *Client, ctx context.Context) error { return c.StartSession2(ctx, INIT_MSG_ID, INIT_RESP_ID) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newT7(t, &fakeT7{closed: tc.closed})
			c.gotSequrityAccess = true
			if err := tc.end(c, t.Context()); err != nil {
				t.Fatal(err)
			}
			if c.gotSequrityAccess {
				t.Fatal("security access still cached")
			}
		})
	}
}
