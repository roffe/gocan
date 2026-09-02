package t7kwp

import (
	"context"
	"fmt"
	"time"
)

// eolPollInterval paces the EOL routine polls. The ECU answers busyRepeatRequest
// (or nothing at all) while a routine runs.
const eolPollInterval = 250 * time.Millisecond

// eraseSettleAttempts bounds the wait for the chip erase to finish after the
// ECU accepts EOLEraseFlash. A T7 chip erase is ~20 s; this allows ~60 s of
// 250 ms polls so a slow or retried erase is waited out rather than written over.
const eraseSettleAttempts = 240

// EraseFlash runs the EOL erase: startRoutine EOLProgrammingStart (0x52), then
// EOLEraseFlash (0x53) — a chip-wide erase that takes ~20 s — then polls
// testerPresent until the ECU answers again. tick, when non-nil, is called once
// per poll so callers can drive a progress bar.
//
// The erase is chip-wide, so afterwards requestDownload may target any address.
func (t *Client) EraseFlash(ctx context.Context, tick func()) error {
	if tick == nil {
		tick = func() {}
	}
	if err := t.pollRoutine(ctx, RLI_EOL_START, 30, tick); err != nil {
		return fmt.Errorf("EraseFlash: start EOL programming: %w", err)
	}
	if err := t.pollRoutine(ctx, RLI_ERASE, 200, tick); err != nil {
		return fmt.Errorf("EraseFlash: erase flash: %w", err)
	}

	// Settle: wait for testerPresent to come back before starting the download.
	// This is a weak check — EOLCommCntrl answers TESTER_PRESENT at the top of
	// the function, so it can pass while the background erase is still running.
	// What actually makes the download safe is retryOnBusy in requestDownload;
	// this loop only catches an ECU that has gone silent altogether.
	// (Measured erase time varies with the flash device: ~4 s on some ECUs, ~20 s
	// on others, so elapsed time is not a usable completion signal either.)
	var err error
	for range eraseSettleAttempts {
		tick()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(eolPollInterval):
		}
		if err = t.TesterPresent(ctx); err == nil {
			return nil
		}
	}
	return fmt.Errorf("EraseFlash: ECU still busy after erase: %w", err)
}

// pollRoutine retries startRoutine until the ECU answers positively.
func (t *Client) pollRoutine(ctx context.Context, id byte, attempts int, tick func()) error {
	var err error
	for range attempts {
		tick()
		if err = t.StartRoutineByIdentifier(ctx, id); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(eolPollInterval):
		}
	}
	return err
}

// EndEOL closes an EOL programming session (startRoutine 0x54). The ECU verifies
// the ROM checksum against the stored PI-area value and sets the EOL-success
// flag; it is only needed for a full EOL run, not an image reflash.
//
// The ECU only accepts it once its procedurecheck mask is complete
// (ALL_CHECKED) — erase, flashcode, transfer start/exit, Delco HW numbers, VIN,
// programming date and tester serial. Anything missing yields
// conditionsNotCorrect; on any failure the routine results are appended raw, as
// the status byte layout for 0x54 is not pinned down.
func (t *Client) EndEOL(ctx context.Context) error {
	err := retryOnBusy(ctx, func() error { return t.StartRoutineByIdentifier(ctx, RLI_END_EOL) })
	if err == nil {
		return nil
	}
	if res, rerr := t.RequestRoutineResultsByLocalIdentifier(ctx, RLI_END_EOL); rerr == nil {
		return fmt.Errorf("EndEOL: %w (routine results: % X)", err, res)
	}
	return fmt.Errorf("EndEOL: %w", err)
}
