package t7kwp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan/v2"
)

// Fast raw transfer: a firmware extension (T7 eu03_erik_org_modified, not in
// any SAAB release; ECU side documented in Vios.77/Source/DIA_VBUS.H). Data
// frames carry 8 raw bytes with no KWP framing and no per-frame ack, so a
// transfer costs ~1/8 frame per byte instead of a round trip per 6 bytes. The
// old services are untouched; FastInfo tells whether an ECU has it.
const (
	FAST_DOWN_ID uint32 = 0x7C0 // tester -> ECU data frames (EOL download)
	FAST_UP_ID   uint32 = 0x7C1 // ECU -> tester data frames (upload)

	RLI_FAST_INFO     = 0x60
	RLI_FAST_UPLOAD   = 0x61
	RLI_FAST_DOWNLOAD = 0x62
	RLI_FAST_COMMIT   = 0x63

	fastVersion = 1
)

// fastIdleTimeout bounds the gap between frames of a fast upload. The ECU sends
// one every ~0.3 ms, so anything near this means a stalled stream.
var fastIdleTimeout = 500 * time.Millisecond

// FastSum is the checksum both ends compute over the data bytes: a
// position-weighted (Fletcher-style) sum, uint32 wrapping. Unlike a plain sum
// it changes when a frame is duplicated and another dropped.
func FastSum(b []byte) uint32 {
	var s1, s2 uint32
	for _, c := range b {
		s1 += uint32(c)
		s2 += s1
	}
	return s2
}

// FastInfo probes for the fast transfer routines with startRoutine 0x60. Stock
// firmware refuses it with a negative response: ok is false and err nil. block
// is the largest FastDownloadBlock the ECU accepts.
//
// Probe in a normal session, before EOLProgrammingStart: the EOL code that runs
// from RAM is the firmware currently in flash, so this also tells whether the
// flash can use FastDownloadBlock. A stock ECU in EOL aborts on the probe.
func (t *Client) FastInfo(ctx context.Context) (block int, ok bool, err error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, START_ROUTINE_BY_IDENTIFIER, RLI_FAST_INFO}, DefaultTimeout, t.responseID)
	if err != nil {
		return 0, false, fmt.Errorf("FastInfo: %w", err)
	}
	_ = t.Ack(ctx, resp.Data[0], false)
	if resp.Data[3] == 0x7F {
		return 0, false, nil
	}
	if err := routineResponseErr(resp, RLI_FAST_INFO); err != nil {
		return 0, false, fmt.Errorf("FastInfo: %w", err)
	}
	if resp.Data[2] < 5 || resp.Data[5] != fastVersion {
		return 0, false, nil // unknown layout: treat as absent
	}
	return int(resp.Data[6])<<8 | int(resp.Data[7]), true, nil
}

// FastUpload reads length bytes from address with startRoutine 0x61: the ECU
// streams them as raw 8-byte frames on FAST_UP_ID (last one zero padded), then
// answers 71 61 with the FastSum of the data. Needs security access level 05
// and a stopped engine. A dropped, duplicated or stale frame fails the read;
// resend it. Any new request stops a running upload in the ECU.
func (t *Client) FastUpload(ctx context.Context, address, length uint32) ([]byte, error) {
	if length == 0 || length > 0xFFFFFF {
		return nil, fmt.Errorf("FastUpload: length 0x%X out of range", length)
	}
	frames := int(length+7) / 8

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// one subscription for data and response keeps their bus order, sized for
	// the whole transfer so a host that falls behind loses nothing
	ch := t.c.SubscribeN(sctx, frames+16, FAST_UP_ID, t.responseID)

	msg := []byte{0x08, START_ROUTINE_BY_IDENTIFIER, RLI_FAST_UPLOAD,
		byte(address >> 16), byte(address >> 8), byte(address),
		byte(length >> 16), byte(length >> 8), byte(length)}
	for _, f := range t.splitRequest2(msg) {
		if err := t.c.Send(ctx, f.frame); err != nil {
			return nil, fmt.Errorf("FastUpload: %w", err)
		}
	}

	out := make([]byte, 0, frames*8)
	idle := time.NewTimer(fastIdleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-idle.C:
			return nil, fmt.Errorf("FastUpload: timeout after %d of %d bytes", len(out), length)
		case f, ok := <-ch:
			if !ok {
				return nil, errors.New("FastUpload: subscription closed")
			}
			idle.Reset(fastIdleTimeout)
			if f.ID == FAST_UP_ID {
				out = append(out, f.Data[:]...)
				continue
			}
			if err := checkErr(f); err != nil {
				_ = t.Ack(ctx, f.Data[0], false)
				return nil, err
			}
			if f.Data[0]&0x40 == 0 || f.Data[3] != START_ROUTINE_BY_IDENTIFIER|0x40 || f.Data[4] != RLI_FAST_UPLOAD {
				_ = t.Ack(ctx, f.Data[0], false)
				return nil, fmt.Errorf("FastUpload: unexpected response: %s", f.String())
			}
			sum, err := t.recvChunked(ctx, f, 5, true)
			if err != nil {
				return nil, fmt.Errorf("FastUpload: checksum: %w", err)
			}
			if len(sum) != 4 {
				return nil, fmt.Errorf("FastUpload: short checksum % X", sum)
			}
			if len(out) != frames*8 {
				return nil, fmt.Errorf("FastUpload: got %d data frames, want %d", len(out)/8, frames)
			}
			out = out[:length]
			if got, want := FastSum(out), binary.BigEndian.Uint32(sum); got != want {
				return nil, fmt.Errorf("FastUpload: checksum %08X, ECU says %08X", got, want)
			}
			return out, nil
		}
	}
}

// FastDownloadBlock programs one block during EOL, after EraseFlash, with
// startRoutine 0x62 (arm: address, length), the data as raw frames on
// FAST_DOWN_ID, then 0x63 (commit: FastSum). The ECU programs the block only
// when byte count and checksum match, and receives the next block while this
// one programs. address and len(data) must be even and len(data) at most the
// FastInfo block.
//
// On error the block was either not programmed or programmed with this same
// data (commit reply lost), so resending it is always safe: flash only clears
// bits, and identical data clears none.
func (t *Client) FastDownloadBlock(ctx context.Context, address uint32, data []byte) error {
	n := uint32(len(data))
	if n == 0 || (address|n)&1 != 0 {
		return fmt.Errorf("FastDownloadBlock: 0x%X+0x%X must be even and non-empty", address, n)
	}
	arm := []byte{0x08, START_ROUTINE_BY_IDENTIFIER, RLI_FAST_DOWNLOAD,
		byte(address >> 16), byte(address >> 8), byte(address),
		byte(n >> 16), byte(n >> 8), byte(n)}
	// busy right after the erase, while the ECU reprograms its flash algorithms
	if err := retryOnBusy(ctx, func() error { return t.fastRoutine(ctx, arm, RLI_FAST_DOWNLOAD) }); err != nil {
		return fmt.Errorf("FastDownloadBlock: arm 0x%X: %w", address, err)
	}
	for i := 0; i < len(data); i += 8 {
		var buf [8]byte
		copy(buf[:], data[i:])
		if err := t.c.Send(ctx, gocan.NewFrame(FAST_DOWN_ID, buf[:])); err != nil {
			return fmt.Errorf("FastDownloadBlock: 0x%X: %w", address+uint32(i), err)
		}
	}
	s := FastSum(data)
	commit := []byte{0x06, START_ROUTINE_BY_IDENTIFIER, RLI_FAST_COMMIT, byte(s >> 24), byte(s >> 16), byte(s >> 8), byte(s)}
	// busy while the previous block is still programming
	if err := retryOnBusy(ctx, func() error { return t.fastRoutine(ctx, commit, RLI_FAST_COMMIT) }); err != nil {
		return fmt.Errorf("FastDownloadBlock: commit 0x%X: %w", address, err)
	}
	return nil
}

// fastRoutine sends a multi-frame startRoutine request (only its last frame is
// answered) and checks the 71 <id> reply.
func (t *Client) fastRoutine(ctx context.Context, msg []byte, id byte) error {
	frames := t.splitRequest2(msg)
	for _, f := range frames[:len(frames)-1] {
		if err := t.c.Send(ctx, f.frame); err != nil {
			return err
		}
	}
	rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	resp, err := t.c.Request(rctx, frames[len(frames)-1].frame, t.responseID)
	cancel()
	if err != nil {
		return err
	}
	_ = t.Ack(ctx, resp.Data[0], false)
	return routineResponseErr(resp, id)
}
