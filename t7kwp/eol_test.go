package t7kwp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/roffe/gocan/v2"
)

// frame builds a 0x258 response frame: flow byte, 0xA1, KWP length, then payload.
func frame(length byte, payload ...byte) gocan.Frame {
	buf := make([]byte, 8)
	buf[0] = 0xC0
	buf[1] = 0xA1
	buf[2] = length
	copy(buf[3:], payload)
	return gocan.NewFrame(0x258, buf)
}

func TestRetryOnBusy(t *testing.T) {
	// the error shape callers actually see: checkErr wraps the sentinel
	busy := fmt.Errorf("RequestTransferExit: %s %w", "REQUEST_TRANSFER_EXIT", ErrBusyRepeatRequest)

	t.Run("resends while busy", func(t *testing.T) {
		calls := 0
		err := retryOnBusy(t.Context(), func() error {
			calls++
			if calls < 3 {
				return busy
			}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Fatalf("called %d times, want 3", calls)
		}
	})

	t.Run("other errors are not retried", func(t *testing.T) {
		calls := 0
		want := fmt.Errorf("nope: %w", ErrSecurityAccessDeniedOrRequested)
		err := retryOnBusy(t.Context(), func() error { calls++; return want })
		if !errors.Is(err, ErrSecurityAccessDeniedOrRequested) {
			t.Fatalf("got %v, want the original error back", err)
		}
		if calls != 1 {
			t.Fatalf("called %d times, want 1", calls)
		}
	})

	t.Run("cancellation stops the loop", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		err := retryOnBusy(ctx, func() error {
			calls++
			cancel()
			return busy
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Fatalf("called %d times, want 1", calls)
		}
	})
}

func TestRoutineResponseErr(t *testing.T) {
	for _, tt := range []struct {
		name    string
		resp    gocan.Frame
		id      byte
		wantErr string
	}{
		{
			name: "positive with matching echo",
			resp: frame(2, START_ROUTINE_BY_IDENTIFIER|0x40, RLI_ERASE),
			id:   RLI_ERASE,
		},
		{
			// the real failure: a late reply to EOLProgrammingStart accepted as
			// the answer to EOLEraseFlash, so the flash begins mid-erase
			name:    "positive for the previous routine",
			resp:    frame(2, START_ROUTINE_BY_IDENTIFIER|0x40, RLI_EOL_START),
			id:      RLI_ERASE,
			wantErr: "not 0x53",
		},
		{
			name:    "busy repeat request",
			resp:    frame(3, 0x7F, START_ROUTINE_BY_IDENTIFIER, 0x21),
			id:      RLI_ERASE,
			wantErr: "Busy",
		},
		{
			name:    "wrong service",
			resp:    frame(2, TESTER_PRESENT|0x40, RLI_ERASE),
			id:      RLI_ERASE,
			wantErr: "unexpected response",
		},
		{
			// an ECU answering with the bare SID carries no echo to check
			name: "bare positive without echo",
			resp: frame(1, START_ROUTINE_BY_IDENTIFIER|0x40),
			id:   RLI_ERASE,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := routineResponseErr(tt.resp, tt.id)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// staleEOLECU models a T7 left in EOL mode (running from RAM) by a flash that
// failed before its erase finished: the 2 s EOLEndTimer from that run has
// expired, so re-arming EOL with 0x52 is immediately undone (EOL_READY) until an
// EOLCommCntrl call refreshes the timer. Only routine starts and testerPresent
// are modelled; tester acks on 0x266 are ignored.
type staleEOLECU struct {
	bus        *gocan.Bus
	eolSession bool // diagnos.session_type == EOL_SESSION
	ready      bool // eol.u8_cntrl == EOL_READY
	timerFresh bool
	erased     bool
}

func (e *staleEOLECU) Open(_ context.Context, b *gocan.Bus) error { e.bus = b; return nil }
func (e *staleEOLECU) Close() error                               { return nil }

func (e *staleEOLECU) Send(_ context.Context, f gocan.Frame) error {
	if f.ID != REQ_MSG_ID {
		return nil
	}
	nrc := func(code byte) { e.bus.Deliver(frame(3, 0x7F, f.Data[3], code)) }
	switch {
	case f.Data[3] == TESTER_PRESENT:
		e.bus.Deliver(frame(1, TESTER_PRESENT|0x40))
	case f.Data[3] != START_ROUTINE_BY_IDENTIFIER:
	case !e.eolSession && f.Data[4] == RLI_EOL_START:
		// normal switch: busy, StartEOLProgramming in RAM resets to EXPECT_ERASE,
		// then the background loop flips it to EOL_READY if the timer is stale
		e.eolSession, e.ready = true, !e.timerFresh
		nrc(BUSY_REPEAT_REQUEST)
	case !e.eolSession:
		nrc(SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
	case f.Data[4] == RLI_EOL_START: // EOLCommCntrl: refresh timer, always positive
		e.timerFresh = true
		e.bus.Deliver(frame(2, START_ROUTINE_BY_IDENTIFIER|0x40, RLI_EOL_START))
	case f.Data[4] == RLI_ERASE && e.ready: // abort, back to the normal switch
		e.timerFresh, e.eolSession = true, false
		nrc(CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR)
	case f.Data[4] == RLI_ERASE:
		e.erased = true
		e.bus.Deliver(frame(2, START_ROUTINE_BY_IDENTIFIER|0x40, RLI_ERASE))
	}
	return nil
}

func TestEraseFlashRestartsStaleEOL(t *testing.T) {
	ecu := &staleEOLECU{}
	bus, err := gocan.OpenAdapter(t.Context(), ecu)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	c := New(bus)
	c.SetResponseID(0x258)

	if err := c.EraseFlash(t.Context(), nil); err != nil {
		t.Fatalf("EraseFlash: %v", err)
	}
	if !ecu.erased {
		t.Fatal("erase never started")
	}
}
