package t7kwp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/roffe/gocan/v2"
)

const (
	INIT_MSG_ID        uint32 = 0x220 // 220h
	INIT_RESP_ID       uint32 = 0x238 // 238h
	REQ_MSG_ID         uint32 = 0x240 // 240h
	RESP_CHUNK_CONF_ID uint32 = 0x266 // 266h
	REQ_CHUNK_CONF_ID  uint32 = 0x270 // 270h
)

const (
	NORMAL_MODE = 0
	DEBUG_MODE  = 1
	SILENT_MODE = 2
)

type Client struct {
	c                 *gocan.Bus
	responseID        uint32
	gotSequrityAccess bool
	seedKey           *SeedKey // optional custom pair, tried before KnownSeedKeys
}

var DefaultTimeout = 200 * time.Millisecond

func New(c *gocan.Bus) *Client {
	return &Client{c: c}
}

// SetSeedKey sets an optional custom seed/key pair, tried before the stock
// KnownSeedKeys in RequestSecurityAccess. A nil pair clears it.
func (t *Client) SetSeedKey(sk *SeedKey) {
	t.seedKey = sk
}

func (t *Client) SetResponseID(id uint32) {
	t.responseID = id
}

// request sends payload on id and waits for a reply on replyID, bounded by timeout.
func (t *Client) request(ctx context.Context, id uint32, payload []byte, timeout time.Duration, replyID uint32) (gocan.Frame, error) {
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return t.c.Request(rctx, newFrame(id, payload), replyID)
}

// newFrame builds a full 8-byte T7 frame. The ECU reads a fixed-size buffer, and
// both TrionicCANLib and the flash path always send 8 bytes, so short fixed
// requests are zero-padded rather than sent with a small DLC. Multi-frame
// payloads built by splitRequest keep their natural length.
func newFrame(id uint32, payload []byte) gocan.Frame {
	var buf [8]byte
	copy(buf[:], payload)
	return gocan.NewFrame(id, buf[:])
}

func (t *Client) StartSession(ctx context.Context, id, responseID uint32) error {
	resp, err := t.request(ctx, id, []byte{0x3F, START_COM_REQ, 0x00, 0x11, byte(REQ_MSG_ID >> 8), byte(REQ_MSG_ID & 0xFF)}, DefaultTimeout, responseID)
	if err != nil {
		return fmt.Errorf("StartSession[1]: %w", err)
	}
	if resp.Data[3] != START_COM_REQ|0x40 {
		return fmt.Errorf("StartSession[2]: %w", TranslateErrorCode(GENERAL_REJECT))
	}
	t.responseID = uint32(resp.Data[6])<<8 | uint32(resp.Data[7])
	// log.Printf("ECU ID: 0x%03X", t.responseID)
	return nil
}

func (t *Client) StartSession2(ctx context.Context, id, responseID uint32) error {
	resp, err := t.request(ctx, id, []byte{0x3F, START_COM_REQ, 0x00, 0x11, byte(0x740 >> 8), byte(0x740 & 0xFF)}, DefaultTimeout, responseID)
	if err != nil {
		return fmt.Errorf("StartSession[1]: %w", err)
	}
	if resp.Data[3] != START_COM_REQ|0x40 {
		return fmt.Errorf("StartSession[2]: %w", TranslateErrorCode(GENERAL_REJECT))
	}
	t.responseID = uint32(resp.Data[6])<<8 | uint32(resp.Data[7])
	// log.Printf("ECU ID: 0x%03X", t.responseID)
	return nil
}

// StopSession ends the diagnostic session (stopCommunication, service 0x82).
// The KWP length is 1: the service has no parameters.
func (t *Client) StopSession(ctx context.Context) error {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, STOP_COM_REQ}, DefaultTimeout, t.responseID)
	if err != nil {
		return fmt.Errorf("StopSession: %w", err)
	}
	_ = t.Ack(ctx, resp.Data[0], false)
	return checkErr(resp)
}

func (t *Client) TesterPresent(ctx context.Context) error {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, TESTER_PRESENT}, DefaultTimeout, t.responseID)
	if err != nil {
		return fmt.Errorf("TesterPresent: %w", err)
	}
	if err := checkErr(resp); err != nil {
		return err
	}
	if resp.Data[3] != TESTER_PRESENT|0x40 {
		return fmt.Errorf("TesterPresent: unexpected response: %s", resp.String())
	}
	return nil
}

func (t *Client) StartRoutineByIdentifier(ctx context.Context, id byte, extra ...byte) error {
	payload := []byte{0x40, 0xA1, 0x03, START_ROUTINE_BY_IDENTIFIER, id}
	payload = append(payload, extra...)
	payload[2] = byte(len(payload) - 3)

	resp, err := t.request(ctx, REQ_MSG_ID, payload, DefaultTimeout*2, t.responseID)
	if err != nil {
		return fmt.Errorf("StartRoutineByIdentifier: %w", err)
	}
	_ = t.Ack(ctx, resp.Data[0], false)
	return routineResponseErr(resp, id)
}

// routineResponseErr validates a startRoutineByLocalIdentifier response.
//
// The routine-id echo matters more than it looks. Every routine answers the
// same positive SID 0x71, so without checking the echo a late reply to one
// routine satisfies the poll for the next one. That is exactly how the EOL
// erase used to finish in 4 seconds: pollRoutine sends 0x52 until it answers,
// then immediately starts polling 0x53, and a 0x52 reply that arrived after its
// own timeout was accepted as "erase complete" — so the download began while
// the chip was still erasing and every request came back busyRepeatRequest.
//
// The check is guarded on the KWP length so an ECU that answers with the bare
// SID (length 1) is still accepted.
func routineResponseErr(resp gocan.Frame, id byte) error {
	if err := checkErr(resp); err != nil {
		return err
	}
	// the EOL erase poll depends on this: the ECU answers busyRepeatRequest, or
	// something else entirely, until the routine actually starts
	if resp.Data[3] != START_ROUTINE_BY_IDENTIFIER|0x40 {
		return fmt.Errorf("StartRoutineByIdentifier: unexpected response: %s", resp.String())
	}
	if resp.Data[2] >= 2 && resp.Data[4] != id {
		return fmt.Errorf("StartRoutineByIdentifier: response is for routine 0x%02X, not 0x%02X: %s", resp.Data[4], id, resp.String())
	}
	return nil
}

// recvChunked collects the payload of a multi-frame KWP response. resp is the
// already-received (and checkErr'd) first frame; firstOffset is where its
// payload starts — 5 for most services, 7 for readMemoryByAddress whose reply
// echoes the address first. The KWP length byte covers the header bytes between
// index 3 and the payload, so both the length adjustment and the first frame's
// payload capacity follow from firstOffset. Each 0x266 ack is itself a request
// whose reply is the next 0x258 chunk (up to 6 bytes at index 2); the low 6
// bits of byte 0 count the frames still to come. ackLast confirms the final
// frame (optional on the wire — the flash/dump paths do, the logger paths
// historically don't).
func (t *Client) recvChunked(ctx context.Context, resp gocan.Frame, firstOffset int, ackLast bool) ([]byte, error) {
	remaining := max(0, int(resp.Data[2])-(firstOffset-3))
	out := make([]byte, 0, remaining)
	n := min(remaining, 8-firstOffset)
	out = append(out, resp.Data[firstOffset:firstOffset+n]...)
	remaining -= n

	for resp.Data[0]&0x3F != 0 {
		var err error
		resp, err = t.request(ctx, RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, resp.Data[0] &^ 0x40}, DefaultTimeout, t.responseID)
		if err != nil {
			return nil, err
		}
		n := min(remaining, 6)
		out = append(out, resp.Data[2:2+n]...)
		remaining -= n
	}
	if ackLast {
		_ = t.Ack(ctx, resp.Data[0], false) // final frame acked with no reply expected
	}
	return out, nil
}

// ReadECUIdentification reads one ECU identification field (service 0x1A) — the
// PI-area entries: VIN 0x90, part number 0x91, engine type 0x97 and friends.
func (t *Client) ReadECUIdentification(ctx context.Context, id byte) ([]byte, error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, READ_ECU_IDENTIFICATION, id}, DefaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("ReadECUIdentification[1]: %w", err)
	}
	if err := checkErr(resp); err != nil {
		return nil, err
	}
	out, err := t.recvChunked(ctx, resp, 5, true)
	if err != nil {
		return nil, fmt.Errorf("ReadECUIdentification[2]: %w", err)
	}
	return out, nil
}

func (t *Client) StopRoutineByIdentifier(ctx context.Context, id byte) ([]byte, error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, STOP_ROUTINE_BY_IDENTIFIER, id}, DefaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("StopRoutineByIdentifier: %w", err)
	}
	log.Println(resp.String())
	return resp.Bytes(), checkErr(resp)
}

func (t *Client) RequestRoutineResultsByLocalIdentifier(ctx context.Context, id byte) ([]byte, error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER, id}, DefaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("RequestRoutineResultsByLocalIdentifier: %w", err)
	}

	log.Println(resp.String())
	return resp.Bytes(), checkErr(resp)
}

func (t *Client) ReadDataByLocalIdentifierMode(ctx context.Context, id, mode byte) ([]byte, error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x03, READ_DATA_BY_IDENTIFIER, id, mode}, DefaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByLocalIdentifier2: %w", err)
	}
	if err := checkErr(resp); err != nil {
		return nil, err
	}
	return t.recvChunked(ctx, resp, 5, false)
}

func (cl *Client) ReadDataByIdentifier(ctx context.Context, id byte) ([]byte, error) {
	resp, err := cl.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, READ_DATA_BY_IDENTIFIER, id}, DefaultTimeout, cl.responseID)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByIdentifier[1]: %w", err)
	}
	if err := checkErr(resp); err != nil {
		return nil, err
	}

	out, err := cl.recvChunked(ctx, resp, 5, false)
	if err != nil {
		return nil, fmt.Errorf("ReadDataByIdentifier[2]: %w", err)
	}
	return out, nil
}

func (t *Client) TransferData(ctx context.Context, length uint32) ([]byte, error) {
	buff := bytes.NewBuffer(nil)
outer:
	for {
		b, err := t.transferData(ctx)
		if err != nil {
			//if err.Error() == "incorrect byte count during block transfer" {
			//	return buff.Bytes(), nil
			//}
			return nil, fmt.Errorf("TransferData[1]: %w", err)
		}
		// C0 BF 04 76 31 50 7600
		// log.Printf("transfer data: %X, size: %d", b, b[2])
		// fmt.Printf("%X\n", b)

		toRead := b[2]
		//		log.Printf("toRead %d, %02X", toRead, b[0])
		if toRead >= 5 {
			buff.WriteByte(b[7])
			toRead -= 5
		}

		if b[0] == 0x80 || b[0] == 0xC0 {
			if err := t.Ack(ctx, b[0], false); err != nil {
				return nil, fmt.Errorf("TransferData[2]: %w", err)
			}
			break
		}

		sctx, cancel := context.WithCancel(ctx)
		ch := t.c.Subscribe(sctx, 0x258)
		if err := t.Ack(ctx, b[0], true); err != nil {
			cancel()
			return nil, fmt.Errorf("TransferData[3]: %w", err)
		}
		for toRead > 0 {
			select {
			case f, ok := <-ch:
				if !ok {
					cancel()
					return nil, fmt.Errorf("TransferData[4]: subscription closed")
				}
				// log.Printf("toRead %d, %X", toRead, d)
				readThis := int(min(6, toRead))
				buff.Write(f.Data[2 : 2+readThis])
				toRead -= byte(readThis)
				if f.Data[0] == 0x80 || f.Data[0] == 0xC0 {
					if err := t.Ack(ctx, f.Data[0], false); err != nil {
						cancel()
						return nil, fmt.Errorf("TransferData[4]: %w", err)
					}
				} else {
					if err := t.Ack(ctx, f.Data[0], true); err != nil {
						cancel()
						return nil, fmt.Errorf("TransferData[5]: %w", err)
					}
				}
				if buff.Len() == int(length) {
					cancel()
					break outer
				}
			case <-time.After(250 * time.Millisecond):
				cancel()
				return nil, fmt.Errorf("TransferData[6]: timeout waiting for data")
			}
		}
		cancel()
	}
	return buff.Bytes(), nil
}

func (t *Client) transferData(ctx context.Context) ([]byte, error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, TRANSFER_DATA}, DefaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("transferData: %w", err)
	}
	return resp.Bytes(), checkErr(resp)
}

const (
	// busyRetryInterval/busyRetries bound a resend loop for NRC 0x21. Programming
	// one flash block takes single-digit milliseconds, so the poll is short; the
	// budget still covers a full chip erase in case the ECU is busy with that.
	busyRetryInterval = 50 * time.Millisecond
	busyRetries       = 600
)

// retryOnBusy resends while the ECU answers busyRepeatRequest (NRC 0x21). That
// code is not a failure — it is the T7 saying "my background flash programmer is
// still writing the previous block, send it again", and both the spec and
// XEolprg.c's `eol.u8_command == PROGRAM` branches expect the tester to do
// exactly that. requestDownload and requestTransferExit are the two that hit it,
// because both immediately follow a transferData whose block is still being
// programmed. Both are idempotent, so resending is safe. transferData hits it
// too — see TransferDataBlock.
func retryOnBusy(ctx context.Context, do func() error) error {
	var err error
	for range busyRetries {
		if err = do(); !errors.Is(err, ErrBusyRepeatRequest) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(busyRetryInterval):
		}
	}
	return err
}

func (t *Client) RequestTransferExit(ctx context.Context) error {
	return retryOnBusy(ctx, func() error {
		resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x01, REQUEST_TRANSFER_EXIT}, DefaultTimeout, t.responseID)
		if err != nil {
			return fmt.Errorf("RequestTransferExit: %w", err)
		}
		_ = t.Ack(ctx, resp.Data[0], false)

		if err := checkErr(resp); err != nil {
			return err
		}

		if resp.Data[3] != REQUEST_TRANSFER_EXIT|0x40 {
			return fmt.Errorf("RequestTransferExit: expected 0x77, got %02X", resp.Data[3])
		}
		return nil
	})
}

// transferDataFrames splits a transferData block into the frames a T7 expects:
// the first carries the KWP header plus 4 payload bytes, the rest 6 each, and
// byte 0 counts down so the ECU knows how many frames still follow.
func transferDataFrames(data []byte) [][8]byte {
	// the first frame always carries 4 payload bytes; pad so a short block does
	// not read past the caller's slice. The ECU consumes only the length byte's
	// worth, so the padding never reaches flash.
	src := make([]byte, max(len(data), 4))
	copy(src, data)

	rows := (len(data) + 3) / 6
	frames := make([][8]byte, 0, rows+1)
	for i, pos := rows, 0; i >= 0; i-- {
		var buf [8]byte
		buf[0], buf[1] = byte(i), 0xA1
		if i == rows { // first frame: KWP header + 4 payload bytes
			buf[0] |= 0x40
			buf[2] = byte(len(data) + 1) // KWP length: service id + data
			buf[3] = TRANSFER_DATA
			pos += copy(buf[4:], src[pos:min(pos+4, len(src))])
		} else {
			pos += copy(buf[2:], src[pos:min(pos+6, len(src))])
		}
		frames = append(frames, buf)
	}
	return frames
}

// TransferDataBlock sends one transferData (0x36) block: the whole block is a
// single KWP message split across frames, and only the last frame is answered
// (0x76 on the response id). Blocks are limited to 254 bytes by the one-byte
// KWP length; the flash path uses 128 to match TrionicCANLib.
//
// busyRepeatRequest is answered by resending the identical block, never by
// re-anchoring with requestDownload. A busy ECU keeps the refused block in its
// programming buffer and points its receive buffer at a trash buffer, then
// programs the kept copy when the resend arrives. A requestDownload sent
// instead is read from that kept buffer: the block's first data bytes become
// the download address. That aborted EOL on a real flash, when a short tail
// block (2 frames) caught the ECU still programming the 128-byte block before it.
func (t *Client) TransferDataBlock(ctx context.Context, data []byte) error {
	if len(data) == 0 || len(data) > 254 {
		return fmt.Errorf("TransferDataBlock: block must be 1-254 bytes, got %d", len(data))
	}
	frames := transferDataFrames(data)
	return retryOnBusy(ctx, func() error { return t.transferDataBlock(ctx, frames) })
}

func (t *Client) transferDataBlock(ctx context.Context, frames [][8]byte) error {
	var resp gocan.Frame
	for i, buf := range frames {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if i < len(frames)-1 {
			if err := t.c.Send(ctx, gocan.NewFrame(REQ_MSG_ID, buf[:])); err != nil {
				return fmt.Errorf("TransferDataBlock: frame %d/%d: %w", i+1, len(frames), err)
			}
			continue
		}
		// last frame: Request registers the response subscription before sending,
		// so a fast ECU reply can't slip through between the send and the receive
		r, err := t.request(ctx, REQ_MSG_ID, buf[:], DefaultTimeout, t.responseID)
		if err != nil {
			return fmt.Errorf("TransferDataBlock: frame %d/%d: %w", i+1, len(frames), err)
		}
		resp = r
	}

	_ = t.Ack(ctx, resp.Data[0], false)

	if err := checkErr(resp); err != nil {
		return err
	}
	if resp.Data[3] != TRANSFER_DATA|0x40 {
		return fmt.Errorf("TransferDataBlock: ECU did not confirm write: %s", resp.String())
	}
	return nil
}

func (t *Client) ClearDynamicallyDefineLocalId(ctx context.Context) error {
	// Message is [0x2C, 0xF0, DM_CDDLI, pad]: SetupDynamicRegister (YDiagb.c)
	// requires the register ID 0xF0 after the SID, and the pad byte makes
	// NoOfBytes (= len-3) 1 so the DM_CDDLI entry loop actually runs and
	// clears all NREG register slots. Malformed variants of this message get
	// routed to J1979 mode $04 (clear emission data) and wipe the fuel
	// adaption — assert the 0x6C echo so that can never pass silently.
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x04, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, DM_CDDLI, 0x00}, DefaultTimeout*2, t.responseID)
	if err != nil {
		return fmt.Errorf("ClearDynamicallyDefineLocalId: %w", err)
	}
	if err := checkErr(resp); err != nil {
		return err
	}
	if resp.Data[3] != DYNAMICALLY_DEFINE_IDENTIFIER|0x40 {
		return fmt.Errorf("ClearDynamicallyDefineLocalId: expected 0x6C, got %02X", resp.Data[3])
	}
	return nil
}

func (t *Client) DynamicallyDefineLocalIdBySymbolNumber(ctx context.Context, index int, symbolNumber int) error {
	return t.sendDDL(ctx, []byte{0x08, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, DM_DBMA, byte(index), 0x00, 0x80, byte(symbolNumber >> 8), byte(symbolNumber)})
}

func (t *Client) DynamicallyDefineLocalIdByAddress(ctx context.Context, index int, address uint32, length uint16) error {
	return t.sendDDL(ctx, []byte{0x08, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, DM_DBMA, byte(index), byte(length), byte((address >> 16) & 0xFF), byte((address >> 8) & 0xFF), byte(address & 0xFF)})
}

func (t *Client) DynamicallyDefineLocalIdByLocID(ctx context.Context, index int, locID int) error {
	return t.sendDDL(ctx, []byte{0x06, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, DM_DBLI, byte(index), 0x00, byte(locID), 0x00})
}

func (t *Client) sendDDL(ctx context.Context, payload []byte) error {
	for _, msg := range t.splitRequest(payload, false) {
		if !msg.rr {
			if err := t.c.Send(ctx, msg.frame); err != nil {
				return fmt.Errorf("DynamicallyDefineLocalIdRequest[1]: %w", err)
			}
		} else {
			rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
			resp, err := t.c.Request(rctx, msg.frame, REQ_CHUNK_CONF_ID)
			cancel()
			if err != nil {
				return fmt.Errorf("DynamicallyDefineLocalIdRequest[2]: %w", err)
			}
			if err := TranslateErrorCode(resp.Data[3+2]); err != nil {
				return fmt.Errorf("DynamicallyDefineLocalIdRequest[3]: %w", err)
			}
		}
	}
	return nil
}

// SeedKey is a securityAccess (service 0x27) algorithm: key = ((seed<<2) ^ XOR) - Sub.
type SeedKey struct{ XOR, Sub uint16 }

// KnownSeedKeys are the XOR/SUB pairs found in stock T7 firmware, tried in
// order. An ECU flashed with a patched algorithm answers to none of them —
// recover its pair from the binary with pkg/widgets/seedkey.
var KnownSeedKeys = []SeedKey{
	{XOR: 0x8142, Sub: 0x2356}, // newer SAAB bins
	{XOR: 0x4081, Sub: 0x1F6F}, // older SAAB bins
}

// calcKey answers the ECU's seed. uint16 wraps, which is the & 0xFFFF the ECU's
// 68k word ops do.
func calcKey(seed uint16, k SeedKey) uint16 {
	return (seed<<2 ^ k.XOR) - k.Sub
}

// RequestSecurityAccess tries the stock seed/key pairs at development priority.
func (t *Client) RequestSecurityAccess(ctx context.Context, force bool) (bool, error) {
	if t.gotSequrityAccess && !force {
		return true, nil
	}
	keys := KnownSeedKeys
	if t.seedKey != nil {
		keys = append([]SeedKey{*t.seedKey}, KnownSeedKeys...) // custom pair first
	}
	for _, k := range keys {
		ok, err := t.SecurityAccess(ctx, DEVELOPMENT_PRIORITY, k)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		if ok {
			return true, nil
		}
	}
	return false, fmt.Errorf("security access denied")
}

// SecurityAccess runs one seed/key exchange at the given level (0x01/0x03/0x05;
// 0x05 is what grants flash programming on a shipping T7). A rejected key is
// returned as an error, not (false, nil).
func (t *Client) SecurityAccess(ctx context.Context, level byte, k SeedKey) (bool, error) {
	seed, err := t.RequestSeed(ctx, level)
	if err != nil {
		return false, err
	}
	key := calcKey(seed, k)

	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x04, SECURITY_ACCESS, level + 1, byte(key >> 8), byte(key)}, DefaultTimeout*2, t.responseID)
	if err != nil {
		return false, fmt.Errorf("send key: %w", err)
	}
	_ = t.Ack(ctx, resp.Data[0], false)
	if resp.Data[3] == SECURITY_ACCESS|0x40 && resp.Data[5] == 0x34 {
		t.gotSequrityAccess = true
		return true, nil
	}
	if err := checkErr(resp); err != nil {
		return false, err
	}
	return false, fmt.Errorf("invalid response to security access: %s", resp.String())
}

// RequestSeed asks for a securityAccess seed at the given level without
// answering it, leaving the ECU locked.
func (t *Client) RequestSeed(ctx context.Context, level byte) (uint16, error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, SECURITY_ACCESS, level}, DefaultTimeout*2, t.responseID)
	if err != nil {
		return 0, fmt.Errorf("request seed: %w", err)
	}
	_ = t.Ack(ctx, resp.Data[0], false)
	if err := checkErr(resp); err != nil {
		return 0, err
	}
	if resp.Data[3] != SECURITY_ACCESS|0x40 {
		return 0, fmt.Errorf("unexpected seed response: %s", resp.String())
	}
	return uint16(resp.Data[5])<<8 | uint16(resp.Data[6]), nil
}

// Ack confirms a received response frame on 0x266 (0x3F on the 3rd byte).
// expectReply hints buffered (ELM/STN) adapters that another response frame
// follows; the last frame of a response is acked with expectReply false.
func (t *Client) Ack(ctx context.Context, val byte, expectReply bool) error {
	if expectReply {
		ctx = gocan.WithExpectedResponses(ctx, 1)
	}
	return t.c.Send(ctx, newFrame(RESP_CHUNK_CONF_ID, []byte{0x40, 0xA1, 0x3F, val & 0xBF}))
}

// kwpFrame is one chunk of a split KWP request; rr marks chunks the ECU
// confirms on REQ_CHUNK_CONF_ID (v1 ResponseRequired).
type kwpFrame struct {
	frame gocan.Frame
	rr    bool
}

func (t *Client) splitRequest(payload []byte, responseRequired bool) []kwpFrame {
	chunkSize := 6
	msgCount := (len(payload) + chunkSize - 1) / chunkSize

	var results []kwpFrame

	for i := range msgCount {
		start := chunkSize * i
		end := min(start+chunkSize, len(payload))

		count := end - start

		msgData := make([]byte, 2+count)
		flag := 0

		if i == 0 {
			flag |= 0x40 // this is the first data chunk
		}
		if i != msgCount-1 {
			flag |= 0x80 // we want confirmation for every chunk except the last one
		}
		msgData[0] = byte(flag | ((msgCount - i - 1) & 0x3F))
		msgData[1] = 0xA1

		copy(msgData[2:], payload[start:end])

		results = append(results, kwpFrame{
			frame: gocan.NewFrame(REQ_MSG_ID, msgData),
			rr:    flag&0x80 == 0x80 || responseRequired,
		})
	}

	return results
}

func (t *Client) ReadFlash(ctx context.Context, addr, length int) ([]byte, error) {
	readPos := addr
	out := bytes.NewBuffer([]byte{})
	for readPos < addr+length {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			readLength := min((addr+length)-readPos, 0xF0)
			var err error
			for attempt := range 3 {
				if attempt > 0 {
					log.Printf("failed to read memory by address, pos: 0x%X, length: 0x%X, retrying: %v", readPos, readLength, err)
				}
				var b []byte
				if b, err = t.ReadMemoryByAddressF0(ctx, readPos, readLength); err == nil {
					out.Write(b)
					break
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read memory by address, pos: 0x%X, length: 0x%X", readPos, readLength)
			}
			readPos += readLength
		}
	}

	return out.Bytes(), nil
}

func (t *Client) ReadMemoryByAddressF0(ctx context.Context, address, length int) ([]byte, error) {
	// Jump to read adress
	if err := t.c.Send(ctx, gocan.NewFrame(REQ_MSG_ID, []byte{0x41, 0xA1, 0x08, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, 0x03, 0x00, byte(length)})); err != nil {
		return nil, fmt.Errorf("failed to set read length: %w", err)
	}
	f, err := t.request(ctx, REQ_MSG_ID, []byte{0x00, 0xA1, byte((address >> 16) & 0xFF), byte((address >> 8) & 0xFF), byte(address & 0xFF), 0x00, 0x00, 0x00}, DefaultTimeout*3, t.responseID)
	if err != nil {
		return nil, err
	}
	if err := t.Ack(ctx, f.Data[0], false); err != nil {
		return nil, fmt.Errorf("failed to ack jump to address response: %w", err)
	}

	if f.Data[3] != 0x6C || f.Data[4] != 0xF0 {
		if f.Data[3] == 0x7F && f.Data[4] == 0x2C {
			return nil, fmt.Errorf("jump to address failed: %w", TranslateErrorCode(f.Data[5]))
		}
		return nil, fmt.Errorf("failed to jump to 0x%X got response: %s", address, f.String())
	}
	b, err := t.recvData(ctx, length)
	if err != nil {
		return nil, fmt.Errorf("recvData failed: %w", err)
	}

	return b, nil
}

func (t *Client) recvData(ctx context.Context, length int) ([]byte, error) {
	// Initialize variables for tracking received bytes and remaining payload
	var receivedBytes, payloadLeft int
	out := bytes.NewBuffer(nil) // Simplified buffer initialization

	// Subscribe to responseID and send the start transfer command
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := t.c.Subscribe(sctx, t.responseID)
	startTransferCmd := []byte{0x40, 0xA1, 0x02, READ_DATA_BY_IDENTIFIER, 0xF0, 0x00, 0x00, 0x00}
	if err := t.c.Send(gocan.WithExpectedResponses(ctx, 1), gocan.NewFrame(REQ_MSG_ID, startTransferCmd)); err != nil {
		return nil, err
	}

	for receivedBytes < length {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(DefaultTimeout * 4):
			return nil, fmt.Errorf("timeout")
		case frame, ok := <-ch:
			if !ok {
				return nil, fmt.Errorf("recvData: subscription closed")
			}
			// Handle frame based on control byte
			if frame.Data[0]&0x40 == 0x40 { // Start or continuation frame
				// Calculate remaining payload excluding non-payload bytes
				payloadLeft = int(frame.Data[2]) - 2
				for i := 5; i < 8 && payloadLeft > 0 && receivedBytes < length; i++ {
					out.WriteByte(frame.Data[i])
					receivedBytes++
					payloadLeft--
				}
			} else { // Consecutive frame
				for i, max := 0, 6; i < max && receivedBytes < length; i++ {
					out.WriteByte(frame.Data[2+i])
					receivedBytes++
					payloadLeft--
					if payloadLeft == 0 {
						break
					}
				}
			}

			// Check if it's the last frame
			if frame.Data[0] == 0x80 || frame.Data[0] == 0xC0 {
				_ = t.Ack(ctx, frame.Data[0], false)
				return out.Bytes(), nil // Directly return from the case block
			}
			_ = t.Ack(ctx, frame.Data[0], true)
		}
	}
	return out.Bytes(), nil
}

// Reset ECU
func (t *Client) ResetECU(ctx context.Context) error {
	f, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, ECU_RESET, 0x01}, DefaultTimeout, t.responseID)
	if err != nil {
		return err
	}
	if err := checkErr(f); err != nil {
		return err
	}
	if f.Data[3] != 0x51 || f.Data[4] != 0x81 {
		return fmt.Errorf("abnormal ecu reset response: %X", f.Data[3:])
	}
	return nil
}

const maxChunk = 244

func (t *Client) ReadMemoryByAddress(ctx context.Context, address, length int) ([]byte, error) {
	if length <= 0 {
		return []byte{}, nil
	}
	out := make([]byte, length)
	offset := 0
	for offset < length {
		toGet := min(length-offset, maxChunk) // single byte length limit
		n, err := t.readMemoryByAddressInto(ctx, address+offset, byte(toGet), out[offset:])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return out[:offset], fmt.Errorf("ReadMemoryByAddress: no data returned")
		}
		offset += n
	}
	return out, nil
}

// readMemoryByAddressInto reads up to 'length' bytes starting at 'address' directly into dst.
// It returns the number of bytes written into dst.
func (t *Client) readMemoryByAddressInto(ctx context.Context, address int, length byte, dst []byte) (int, error) {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{
		0x40, 0xA1, 0x05, READ_MEMORY_BY_ADDRESS,
		byte(address >> 16), byte(address >> 8), byte(address),
		length,
	}, DefaultTimeout, t.responseID)
	if err != nil {
		return 0, fmt.Errorf("ReadDataByAddress[1]: %w", err)
	}
	if err := checkErr(resp); err != nil {
		return 0, err
	}

	// the reply echoes the address, so the payload starts at index 7
	b, err := t.recvChunked(ctx, resp, 7, false)
	if err != nil {
		return 0, fmt.Errorf("ReadDataByAddress[2]: %w", err)
	}
	return copy(dst, b), nil
}

func (t *Client) WriteDataByAddress(ctx context.Context, address uint32, data []byte) error {
	message := append([]byte{byte(4 + len(data)), WRITE_DATA_BY_ADDRESS, byte(address >> 16), byte(address >> 8), byte(address), byte(len(data))}, data...)
	if err := t.sendLong(ctx, message); err != nil {
		return fmt.Errorf("WriteDataToAddress: %w", err)
	}
	return nil
}

// WriteDataByLocalIdentifier writes one PI-area field (service 0x3B). The EOL
// tail uses it for VIN 0x90, programming date 0x99 and tester serial 0x98; the
// ECU stores it with writePIArea(id, len(data), data).
//
// ⚠️ id 0x98 is checked against a kill-list in the ECU — run the value through
// TesterSerialBlocked before calling this.
func (t *Client) WriteDataByLocalIdentifier(ctx context.Context, id byte, data []byte) error {
	payload := append([]byte{byte(len(data) + 2), WRITE_DATA_BY_LOCAL_IDENTIFIER, id}, data...)
	return retryOnBusy(ctx, func() error { return t.writeDataByLocalIdentifier(ctx, id, payload) })
}

func (t *Client) writeDataByLocalIdentifier(ctx context.Context, id byte, payload []byte) error {
	for _, msg := range t.splitRequest2(payload) {
		if !msg.rr {
			if err := t.c.Send(ctx, msg.frame); err != nil {
				return fmt.Errorf("WriteDataByLocalIdentifier[1]: %w", err)
			}
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, DefaultTimeout*2)
		resp, err := t.c.Request(rctx, msg.frame, t.responseID)
		cancel()
		if err != nil {
			return fmt.Errorf("WriteDataByLocalIdentifier[2] id 0x%02X: %w", id, err)
		}
		_ = t.Ack(ctx, resp.Data[0], false)
		if err := checkErr(resp); err != nil {
			return fmt.Errorf("WriteDataByLocalIdentifier id 0x%02X: %w", id, err)
		}
		if resp.Data[3] != WRITE_DATA_BY_LOCAL_IDENTIFIER|0x40 {
			return fmt.Errorf("WriteDataByLocalIdentifier id 0x%02X: unexpected response: %s", id, resp.String())
		}
	}
	return nil
}

func (t *Client) RequestUpload(ctx context.Context, address, length uint32) error {
	message := []byte{0x07, REQUEST_UPLOAD, byte(address >> 16), byte(address >> 8), byte(address), byte(length >> 16), byte(length >> 8), byte(length)}
	for _, msg := range t.splitRequest(message, false) {
		if !msg.rr {
			rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
			resp, err := t.c.Request(rctx, msg.frame, t.responseID)
			cancel()
			if err != nil {
				return err
			}
			if err := checkErr(resp); err != nil {
				return err
			}

		} else {
			rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
			resp, err := t.c.Request(rctx, msg.frame, REQ_CHUNK_CONF_ID)
			cancel()
			if err != nil {
				return fmt.Errorf("RequestUpload: %w", err)
			}
			if err := TranslateErrorCode(resp.Data[5]); err != nil {
				return fmt.Errorf("RequestUpload: %w", err)
			}
		}
	}

	return nil
}

func (t *Client) sendLong(ctx context.Context, data []byte) error {
	messages := t.splitRequest(data, true)
	for i, msg := range messages {
		replyID := REQ_CHUNK_CONF_ID
		if i == len(messages)-1 {
			replyID = t.responseID
		}
		rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
		resp, err := t.c.Request(rctx, msg.frame, replyID)
		cancel()
		if err != nil {
			return fmt.Errorf("%s2: %w", getFunctionName(), err)
		}
		if err := checkErr(resp); err != nil {
			return err
		}
	}
	return nil
}

func (t *Client) ReadROM(ctx context.Context, address, sramOffset uint32, length uint32) ([]byte, error) {
	if err := t.RequestUpload(ctx, address-sramOffset, length); err != nil {
		return nil, err
	}
	b, err := t.TransferData(ctx, length)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (t *Client) ReadRAM(ctx context.Context, address, length uint32) ([]byte, error) {
	data, err := t.ReadMemoryByAddress(ctx, int(address), int(length))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (t *Client) SaveRAM(ctx context.Context, address uint32, data []byte) error {
	start := time.Now()
	defer func(t time.Time) {
		log.Println("saveRam took:", time.Since(t))
	}(start)
	if err := t.WriteDataByAddress(ctx, address, data); err != nil {
		return err
	}
	return nil
}

func (t *Client) SaveROM(ctx context.Context, address uint32, data []byte) error {
	start := time.Now()

	if err := t.RequestDownload(ctx, address, uint32(len(data))); err != nil {
		return err
	}

	defer func(t time.Time) {
		log.Println("saveROM took:", time.Since(t))
	}(start)

	msgs := t.splitRequest2(append([]byte{byte(len(data) + 1), TRANSFER_DATA}, data...))
	for _, msg := range msgs {
		log.Println(msg.frame.String())
		if !msg.rr {
			if err := t.c.Send(ctx, msg.frame); err != nil {
				return fmt.Errorf("%s1: %w", getFunctionName(), err)
			}
		} else {
			rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
			resp, err := t.c.Request(rctx, msg.frame, REQ_CHUNK_CONF_ID)
			cancel()
			if err != nil {
				return fmt.Errorf("%s2: %w", getFunctionName(), err)
			}
			log.Println(resp.String())
			if err := checkErr(resp); err != nil {
				return err
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
	// return t.writeRange(ctx, int(address), int(address)+len(data), data)
}

func (t *Client) splitRequest2(payload []byte) []kwpFrame {
	msgCount := (len(payload) + 6 - 1) / 6

	left := len(payload)
	var results []kwpFrame

	msgLen := func() int {
		if left >= 6 {
			left -= 6
			return 6
		} else {
			return left
		}
	}

	for i := 0; i < msgCount; i++ {
		count := msgLen()
		msgData := make([]byte, 2+count)
		flag := 0

		if i == 0 {
			flag |= 0x40 // this is the first data chunk
		}
		/*
			if i != msgCount-1 {
				flag |= 0x80 // we want confirmation for every chunk except the last one
			}
		*/
		msgData[0] = byte(flag | (msgCount-i-1)&0x3F) // & 0x3F is not necessary, only to show that this field is 6-bit wide
		msgData[1] = 0xA1

		start := 6 * i

		copy(msgData[2:], payload[start:start+count])
		for j := 0; j < count; j++ {
			msgData[2+j] = payload[start+j]
		}

		results = append(results, kwpFrame{
			frame: gocan.NewFrame(REQ_MSG_ID, msgData),
			rr:    i == msgCount-1,
		})

		/*
			if flag&0x80 == 0x80 {
				results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.ResponseRequired))
			} else {
				results = append(results, gocan.NewFrame(REQ_MSG_ID, msgData, gocan.Outgoing))
			}
		*/
	}
	return results
}

// RequestDownload anchors the ECU's write pointer (service 0x34). The ECU
// accepts a fresh requestDownload mid-stream, so a failed block can resume from
// its failure point instead of restarting.
func (t *Client) RequestDownload(ctx context.Context, address uint32, length uint32) error {
	message := []byte{0x08, REQUEST_DOWNLOAD, byte(address >> 16), byte(address >> 8), byte(address), 0x00, byte(length >> 16), byte(length >> 8), byte(length)}
	return retryOnBusy(ctx, func() error {
		for _, msg := range t.splitRequest2(message) {
			if !msg.rr {
				if err := t.c.Send(ctx, msg.frame); err != nil {
					return fmt.Errorf("RequestDownload[1]: %w", err)
				}
				continue
			}
			rctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
			resp, err := t.c.Request(rctx, msg.frame, t.responseID)
			cancel()
			if err != nil {
				return fmt.Errorf("RequestDownload[2]: %w", err)
			}
			_ = t.Ack(ctx, resp.Data[0], false)
			if err := checkErr(resp); err != nil {
				return err
			}
			if resp.Data[3] != REQUEST_DOWNLOAD|0x40 {
				return fmt.Errorf("RequestDownload[3]: invalid response enabling download mode: %s", resp.String())
			}
		}
		return nil
	})
}

// DTC is one diagnostic trouble code as the T7 reports it: the 2-byte SAE code
// rendered as "Pxxxx" and the raw KWP status byte. T7 DTCs carry no
// failure-type suffix on the wire (unlike GMLAN).
type DTC struct {
	Code   string
	Status byte
}

// ReadDTCByStatus reads stored DTCs (service 0x18). It is a plain read, so a
// timed-out exchange is simply requested again: the ECU drops an unacked reply
// after 400 ms (T1_TIMEOUT, Vios.77 VBUS.H), and a fresh request resets its
// transmit state anyway. One lost or late frame no longer fails the read.
func (t *Client) ReadDTCByStatus(ctx context.Context, status byte) ([]DTC, error) {
	var err error
	for range 3 {
		var dtcs []DTC
		if dtcs, err = t.readDTCByStatus(ctx, status); !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return dtcs, err
		}
	}
	return nil, err
}

func (t *Client) readDTCByStatus(ctx context.Context, status byte) ([]DTC, error) {
	const (
		posRespReadDTCByStatus = 0x58 // KWP2000 positive response SID for "Read DTC by status"
		bytesPerDTC            = 3
	)

	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x02, READ_DTC_BY_STATUS, status}, DefaultTimeout, t.responseID)
	if err != nil {
		return nil, fmt.Errorf("ReadDTCByStatus[0]: %w", err)
	}
	if err := checkErr(resp); err != nil {
		return nil, err
	}
	if resp.Length < 5 {
		return nil, fmt.Errorf("ReadDTCByStatus[1]: short first response: % X", resp.Bytes())
	}
	if resp.Data[3] != posRespReadDTCByStatus {
		return nil, fmt.Errorf("ReadDTCByStatus[2]: unexpected SID 0x%02X (data % X)", resp.Data[3], resp.Data)
	}
	numDTCs := int(resp.Data[4])

	// Payload after the count byte: 3 bytes per DTC (code hi, code lo, status).
	data, err := t.recvChunked(ctx, resp, 5, true)
	if err != nil {
		return nil, fmt.Errorf("ReadDTCByStatus[3]: %w", err)
	}
	if len(data) < numDTCs*bytesPerDTC {
		return nil, fmt.Errorf("ReadDTCByStatus[4]: not enough DTC data: have %d bytes for %d DTCs", len(data), numDTCs)
	}

	var dtcs []DTC
	for d := range slices.Chunk(data[:numDTCs*bytesPerDTC], bytesPerDTC) {
		dtcs = append(dtcs, DTC{Code: fmt.Sprintf("P%02X%02X", d[0], d[1]), Status: d[2]})
	}
	return dtcs, nil
}

func (t *Client) ClearDTCS(ctx context.Context) error {
	resp, err := t.request(ctx, REQ_MSG_ID, []byte{0x40, 0xA1, 0x03, CLEAR_DTC, 0xFF, 0x00}, DefaultTimeout, t.responseID)
	if err != nil {
		return fmt.Errorf("ClearDTCS[0]: %w", err)
	}
	// log.Println(resp.String())
	if err := checkErr(resp); err != nil {
		return err
	}
	// successful response: C0 BF 03 54 FF 00 00 00
	if resp.Data[3]^0x40 != CLEAR_DTC || resp.Data[4] != 0xFF || resp.Data[5] != 0x00 {
		return fmt.Errorf("ClearDTCS[1]: invalid response: % 02X", resp.Data)
	}
	return nil
}

func getFunctionName() string {
	return getFunctionNameN(2)
}

func getFunctionNameN(depth int) string {
	pc, _, _, ok := runtime.Caller(depth)
	if !ok {
		return "unknown"
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}

	return textAfterLastDot(fn.Name())
}

func checkErr(f gocan.Frame) error {
	if f.Data[3] == 0x7F {
		return fmt.Errorf("%s: %s %w", getFunctionNameN(2), TranslateServiceID(f.Data[4]), TranslateErrorCode(f.Data[5]))
	}
	return nil
}

func textAfterLastDot(s string) string {
	lastDotIndex := strings.LastIndex(s, ".")
	if lastDotIndex == -1 || lastDotIndex == len(s)-1 {
		return ""
	}
	return s[lastDotIndex+1:]
}
