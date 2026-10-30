package t7kwp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

func TestRequestDownload(t *testing.T) {
	e := &fakeT7{flash: make([]byte, 0x80000)}
	c, node := newT7(t, e)
	if err := c.RequestDownload(t.Context(), 0x7FE00, 0x200); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{
		{0x41, 0xA1, 0x08, REQUEST_DOWNLOAD, 0x07, 0xFE, 0x00, 0x00},
		{0x00, 0xA1, 0x00, 0x02, 0x00},
	}
	if got := sentOn(node, REQ_MSG_ID); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatalf("sent %02X, want %02X", got, want)
	}
	if got := acks(node); !bytes.Equal(got, []byte{0x80}) {
		t.Fatalf("acks % 02X, want 80", got)
	}
	if e.dlAddr != 0x7FE00 || e.dlEnd != 0x80000 {
		t.Fatalf("ECU anchored at 0x%X-0x%X", e.dlAddr, e.dlEnd)
	}
}

// busyRepeatRequest is answered with the identical request, anything else
// fails at once.
func TestRequestDownloadReplies(t *testing.T) {
	for _, tc := range []struct {
		name      string
		busy      int
		flash     int
		wantErr   error
		wantTries int
	}{
		{name: "busy twice", busy: 2, flash: 0x80000, wantTries: 3},
		{name: "address refused", flash: 0x1000, wantErr: ErrCannotDownloadToSpecifiedAddress, wantTries: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{flash: make([]byte, tc.flash), busy: map[byte]int{REQUEST_DOWNLOAD: tc.busy}}
			c, _ := newT7(t, e)
			checkErrIs(t, c.RequestDownload(t.Context(), 0x7FE00, 0x200), tc.wantErr)
			if got := e.count(REQUEST_DOWNLOAD); got != tc.wantTries {
				t.Fatalf("%d requestDownloads, want %d", got, tc.wantTries)
			}
			for i := range e.requests {
				if !bytes.Equal(e.kwp(i), e.kwp(0)) {
					t.Fatalf("resend % 02X differs from % 02X", e.kwp(i), e.kwp(0))
				}
			}
		})
	}
}

func TestRequestDownloadNoReply(t *testing.T) {
	shortTimeout(t)
	c, _ := newT7(t, &fakeT7{closed: true})
	if err := c.RequestDownload(t.Context(), 0x7FE00, 0x200); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a timeout", err)
	}
}

// A flash run as the ECU sees it: requestDownload, transferData blocks of
// 128 bytes and a short tail, requestTransferExit. The first block and the
// exit catch the ECU busy programming and are resent.
func TestDownloadSequence(t *testing.T) {
	img := testImage(300)
	e := &fakeT7{flash: make([]byte, 0x1000), busy: map[byte]int{TRANSFER_DATA: 1, REQUEST_TRANSFER_EXIT: 1}}
	c, node := newT7(t, e)
	ctx := t.Context()
	if err := c.RequestDownload(ctx, 0x100, uint32(len(img))); err != nil {
		t.Fatal(err)
	}
	for b := range slices.Chunk(img, 128) {
		if err := c.TransferDataBlock(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.RequestTransferExit(ctx); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(e.flash[0x100:0x100+len(img)], img) {
		t.Fatal("flash does not hold the image")
	}
	if !bytes.Equal(e.flash[0x100+len(img):0x200+len(img)], make([]byte, 0x100)) {
		t.Fatal("bytes written past the image")
	}
	if got := e.count(TRANSFER_DATA); got != 4 {
		t.Fatalf("%d transferData, want 3 blocks + 1 resend", got)
	}
	if got := e.count(REQUEST_TRANSFER_EXIT); got != 2 {
		t.Fatalf("%d requestTransferExit, want 1 + 1 resend", got)
	}
	for _, f := range sentOn(node, REQ_MSG_ID) {
		if f[0]&0x80 != 0 {
			t.Fatalf("row % 02X asks for a 0x270 confirmation; download rows are sent back to back", f)
		}
	}
}

func TestTransferDataBlockRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    uint32
		wantErr error
	}{
		{name: "no requestDownload", wantErr: ErrCannotDownloadToSpecifiedAddress},
		{name: "block past the end", size: 4, wantErr: ErrIncorrectByteCountDuringBlockTransfer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{flash: make([]byte, 0x1000)}
			c, _ := newT7(t, e)
			if tc.size > 0 {
				if err := c.RequestDownload(t.Context(), 0x100, tc.size); err != nil {
					t.Fatal(err)
				}
			}
			checkErrIs(t, c.TransferDataBlock(t.Context(), testImage(8)), tc.wantErr)
			if got := e.count(TRANSFER_DATA); got != 1 {
				t.Fatalf("%d transferData, want 1", got)
			}
		})
	}
}

func TestTransferDataBlockNotSent(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		data    []byte
		wantErr error
	}{
		{"empty block", t.Context(), nil, errAny},
		{"255 bytes", t.Context(), testImage(255), errAny},
		{"cancelled", cancelled, testImage(8), context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, node := newT7(t, &fakeT7{flash: make([]byte, 0x1000)})
			checkErrIs(t, c.TransferDataBlock(tc.ctx, tc.data), tc.wantErr)
			if got := node.Sent(); len(got) != 0 {
				t.Fatalf("sent %v", got)
			}
		})
	}
}

// A lost confirmation is a timeout. It is not resent: only busyRepeatRequest
// says the block was not taken.
func TestTransferDataBlockLostReply(t *testing.T) {
	shortTimeout(t)
	e := &fakeT7{flash: make([]byte, 0x1000), mode: REQUEST_DOWNLOAD, dlAddr: 0x100, dlEnd: 0x200}
	node := e.node()
	node.Fault = gocantest.Drop(0)
	if err := newClient(t, node).TransferDataBlock(t.Context(), testImage(20)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if got := e.count(TRANSFER_DATA); got != 1 {
		t.Fatalf("%d transferData, want 1", got)
	}
}

func TestDownloadUnplugged(t *testing.T) {
	unplugged := errors.New("adapter unplugged")
	c, node := newT7(t, &fakeT7{flash: make([]byte, 0x1000)})
	if err := c.RequestDownload(t.Context(), 0x100, 0x10); err != nil {
		t.Fatal(err)
	}
	node.Unplug(unplugged)
	if err := c.TransferDataBlock(t.Context(), testImage(16)); !errors.Is(err, unplugged) {
		t.Fatalf("TransferDataBlock: err = %v, want %v", err, unplugged)
	}
	if err := c.RequestTransferExit(t.Context()); !errors.Is(err, unplugged) {
		t.Fatalf("RequestTransferExit: err = %v, want %v", err, unplugged)
	}
}

func TestRequestTransferExit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		busy      int
		answer    func([]byte) []byte
		wantErr   error
		wantTries int
	}{
		{name: "positive", wantTries: 1},
		{name: "busy twice", busy: 2, wantTries: 3},
		{name: "general reject", answer: answerWith(0x7F, REQUEST_TRANSFER_EXIT, GENERAL_REJECT), wantErr: ErrGeneralReject, wantTries: 1},
		{name: "other service", answer: answerWith(TRANSFER_DATA | 0x40), wantErr: errAny, wantTries: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{busy: map[byte]int{REQUEST_TRANSFER_EXIT: tc.busy}}
			if tc.answer != nil {
				e.answer = map[byte]func([]byte) []byte{REQUEST_TRANSFER_EXIT: tc.answer}
			}
			c, node := newT7(t, e)
			checkErrIs(t, c.RequestTransferExit(t.Context()), tc.wantErr)
			if got := e.count(REQUEST_TRANSFER_EXIT); got != tc.wantTries {
				t.Fatalf("%d requestTransferExit, want %d", got, tc.wantTries)
			}
			if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x01, REQUEST_TRANSFER_EXIT, 0, 0, 0, 0}; !bytes.Equal(got, want) {
				t.Fatalf("sent % 02X, want % 02X", got, want)
			}
			if got := acks(node); len(got) != tc.wantTries || got[0] != 0x80 {
				t.Fatalf("acks % 02X, want one 80 per reply", got)
			}
		})
	}
}

// An ECU that stays busy is given up on when the context ends.
func TestRequestTransferExitDeadline(t *testing.T) {
	c, _ := newT7(t, &fakeT7{busy: map[byte]int{REQUEST_TRANSFER_EXIT: 1 << 20}})
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := c.RequestTransferExit(ctx); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("err = %v after %v, want the context's deadline", err, time.Since(start))
	}
}

func TestWriteDataByLocalIdentifier(t *testing.T) {
	vin := []byte("YS3FF45S1X1234567")
	e := &fakeT7{}
	c, node := newT7(t, e)
	if err := c.WriteDataByLocalIdentifier(t.Context(), 0x90, vin); err != nil {
		t.Fatal(err)
	}
	if want := append([]byte{19, WRITE_DATA_BY_LOCAL_IDENTIFIER, 0x90}, vin...); !bytes.Equal(e.kwp(0), want) {
		t.Fatalf("ECU got % 02X, want % 02X", e.kwp(0), want)
	}
	var heads []byte
	for _, f := range sentOn(node, REQ_MSG_ID) {
		heads = append(heads, f[0])
	}
	if want := []byte{0x43, 0x02, 0x01, 0x00}; !bytes.Equal(heads, want) {
		t.Fatalf("row headers % 02X, want % 02X", heads, want)
	}
	if !bytes.Equal(e.ids[0x90], vin) {
		t.Fatalf("ECU stored %q", e.ids[0x90])
	}
	if got := acks(node); !bytes.Equal(got, []byte{0x80}) {
		t.Fatalf("acks % 02X, want 80", got)
	}
}

func TestWriteDataByLocalIdentifierReplies(t *testing.T) {
	for _, tc := range []struct {
		name      string
		busy      int
		answer    func([]byte) []byte
		wantErr   error
		wantTries int
	}{
		{name: "busy once", busy: 1, wantTries: 2},
		{name: "written twice already", answer: answerWith(0x7F, WRITE_DATA_BY_LOCAL_IDENTIFIER, REQUEST_OUT_OF_RANGE), wantErr: ErrRequestOutOfRange, wantTries: 1},
		{name: "other service", answer: answerWith(REQUEST_TRANSFER_EXIT | 0x40), wantErr: errAny, wantTries: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{busy: map[byte]int{WRITE_DATA_BY_LOCAL_IDENTIFIER: tc.busy}}
			if tc.answer != nil {
				e.answer = map[byte]func([]byte) []byte{WRITE_DATA_BY_LOCAL_IDENTIFIER: tc.answer}
			}
			c, _ := newT7(t, e)
			checkErrIs(t, c.WriteDataByLocalIdentifier(t.Context(), 0x99, []byte("261030")), tc.wantErr)
			if got := e.count(WRITE_DATA_BY_LOCAL_IDENTIFIER); got != tc.wantTries {
				t.Fatalf("%d writes, want %d", got, tc.wantTries)
			}
		})
	}
}

func TestEndEOL(t *testing.T) {
	for _, tc := range []struct {
		name       string
		busy       int
		answer     map[byte]func([]byte) []byte
		wantErr    error
		wantInText string
		wantSIDs   []byte
	}{
		{name: "positive", wantSIDs: []byte{START_ROUTINE_BY_IDENTIFIER}},
		{name: "busy once", busy: 1, wantSIDs: []byte{START_ROUTINE_BY_IDENTIFIER, START_ROUTINE_BY_IDENTIFIER}},
		{
			name: "checksum mismatch with results",
			answer: map[byte]func([]byte) []byte{
				START_ROUTINE_BY_IDENTIFIER:                 answerWith(0x7F, START_ROUTINE_BY_IDENTIFIER, BLOCK_TRANSFER_DATA_CHECKSUM_ERROR),
				REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER: answerWith(REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER|0x40, RLI_END_EOL, 0x0B),
			},
			wantErr:    ErrBlockTransferDataChecksumError,
			wantInText: "routine results: C0 BF 03 73 54 0B",
			wantSIDs:   []byte{START_ROUTINE_BY_IDENTIFIER, REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER},
		},
		{
			name: "refused, no results",
			answer: map[byte]func([]byte) []byte{
				START_ROUTINE_BY_IDENTIFIER:                 answerWith(0x7F, START_ROUTINE_BY_IDENTIFIER, CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR),
				REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER: answerWith(0x7F, REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER, SERVICE_NOT_SUPPORTED),
			},
			wantErr:  ErrConditionsNotCorrectOrRequestSequenceError,
			wantSIDs: []byte{START_ROUTINE_BY_IDENTIFIER, REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{busy: map[byte]int{START_ROUTINE_BY_IDENTIFIER: tc.busy}, answer: tc.answer}
			c, _ := newT7(t, e)
			err := c.EndEOL(t.Context())
			checkErrIs(t, err, tc.wantErr)
			if tc.wantInText != "" && !strings.Contains(err.Error(), tc.wantInText) {
				t.Fatalf("err %q does not carry %q", err, tc.wantInText)
			}
			if !bytes.Equal(e.sids(), tc.wantSIDs) {
				t.Fatalf("requests % 02X, want % 02X", e.sids(), tc.wantSIDs)
			}
			for i := range e.requests {
				if e.requests[i][2] != RLI_END_EOL {
					t.Fatalf("request % 02X is not for routine 0x54", e.kwp(i))
				}
			}
		})
	}
}

func TestRequestUpload(t *testing.T) {
	e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: testImage(0x10000)}
	c, node := newT7(t, e)
	if err := c.RequestUpload(t.Context(), 0x1234, 0x56); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{
		{0xC1, 0xA1, 0x07, REQUEST_UPLOAD, 0x00, 0x12, 0x34, 0x00},
		{0x00, 0xA1, 0x00, 0x56},
	}
	if got := sentOn(node, REQ_MSG_ID); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatalf("sent %02X, want %02X", got, want)
	}
	if e.mode != REQUEST_UPLOAD || e.upAddr != 0x1234 || e.upLeft != 0x56 {
		t.Fatalf("ECU set up upload mode %02X at 0x%X, 0x%X bytes", e.mode, e.upAddr, e.upLeft)
	}
}

func TestRequestUploadRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		access  byte
		addr    uint32
		wantErr error
	}{
		{"locked", NO_PRIORITY, 0x100, ErrSecurityAccessDeniedOrRequested},
		{"out of range", DEVELOPMENT_PRIORITY, 0xFF00, ErrCannotUploadFromSpecifiedAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newT7(t, &fakeT7{access: tc.access, mem: make([]byte, 0x10000)})
			checkErrIs(t, c.RequestUpload(t.Context(), tc.addr, 0x200), tc.wantErr)
		})
	}
}

func TestRequestUploadOtherService(t *testing.T) {
	e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: make([]byte, 0x1000), answer: map[byte]func([]byte) []byte{
		REQUEST_UPLOAD: answerWith(REQUEST_DOWNLOAD|0x40, 0xFF),
	}}
	c, _ := newT7(t, e)
	if err := c.RequestUpload(t.Context(), 0x100, 0x10); err == nil {
		t.Fatal("RequestUpload accepted a 74 reply")
	}
}

// The first row of requestUpload asks for a 0x270 confirmation; without one
// the request fails.
func TestRequestUploadUnconfirmedRow(t *testing.T) {
	shortTimeout(t)
	e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: make([]byte, 0x1000)}
	node := e.node()
	node.Fault = gocantest.Drop(0)
	if err := newClient(t, node).RequestUpload(t.Context(), 0x100, 0x10); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a timeout", err)
	}
}

// ReadROM reads through requestUpload and as many transferData blocks as the
// length takes. 1 and 251 end on a block that fits in a single row.
func TestReadROM(t *testing.T) {
	mem := testImage(0x1000)
	for _, n := range []uint32{1, 5, 16, 250, 251, 600} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: mem}
			c, node := newT7(t, e)
			got, err := c.ReadROM(t.Context(), 0x2100, 0x2000, n)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, mem[0x100:0x100+n]) {
				t.Fatalf("got %d bytes, not mem[0x100:0x%X]", len(got), 0x100+n)
			}
			if blocks := (int(n) + 249) / 250; e.count(TRANSFER_DATA) != blocks {
				t.Fatalf("%d transferData, want %d", e.count(TRANSFER_DATA), blocks)
			}
			if a := acks(node); a[len(a)-1] != 0x80 {
				t.Fatalf("last ack %02X, want 80", a[len(a)-1])
			}
		})
	}
}

func TestTransferDataNegative(t *testing.T) {
	c, _ := newT7(t, &fakeT7{access: DEVELOPMENT_PRIORITY, mode: REQUEST_UPLOAD})
	if _, err := c.TransferData(t.Context(), 4); !errors.Is(err, ErrIncorrectByteCountDuringBlockTransfer) {
		t.Fatalf("err = %v, want %v", err, ErrIncorrectByteCountDuringBlockTransfer)
	}
}

// With length 0 the transfer runs until the ECU sends a block with no data,
// the way the symbol table ends.
func TestTransferDataUntilEmptyBlock(t *testing.T) {
	data := testImage(17)
	blocks := [][]byte{data[:10], data[10:], nil}
	n := 0
	e := &fakeT7{answer: map[byte]func([]byte) []byte{TRANSFER_DATA: func([]byte) []byte {
		b := blocks[min(n, len(blocks)-1)]
		n++
		return append([]byte{TRANSFER_DATA | 0x40, START_ROUTINE_BY_IDENTIFIER, RLI_SYM, byte(n - 1)}, b...)
	}}}
	c, _ := newT7(t, e)
	got, err := c.TransferData(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) || n != 3 {
		t.Fatalf("got % 02X after %d blocks, want % 02X after 3", got, n, data)
	}
}

// uploadECU has an upload of 16 bytes set up: one transferData reply of four rows.
func uploadECU() (*fakeT7, []byte) {
	mem := testImage(0x100)
	return &fakeT7{access: DEVELOPMENT_PRIORITY, mem: mem, mode: REQUEST_UPLOAD, upAddr: 0x40, upLeft: 16}, mem[0x40:0x50]
}

// A lost row ends the transfer after the client's fixed 250 ms row timeout.
func TestTransferDataLostRow(t *testing.T) {
	e, _ := uploadECU()
	node := e.node()
	node.Fault = gocantest.Drop(2)
	if got, err := newClient(t, node).TransferData(t.Context(), 16); err == nil {
		t.Fatalf("got % 02X with no error", got)
	}
}

func TestTransferDataUnderFaults(t *testing.T) {
	for k := 1; k <= 3; k++ {
		for name, fault := range map[string]func(int, gocan.Frame) []gocan.Frame{
			"duplicate": gocantest.Duplicate(k),
			"truncate":  gocantest.Truncate(k, 3),
		} {
			t.Run(fmt.Sprintf("%s row %d", name, k), func(t *testing.T) {
				e, want := uploadECU()
				node := e.node()
				node.Fault = fault
				got, err := newClient(t, node).TransferData(t.Context(), 16)
				if err == nil && !bytes.Equal(got, want) {
					t.Fatalf("got % 02X with no error, want % 02X", got, want)
				}
			})
		}
	}
}

func TestTransferDataCancelled(t *testing.T) {
	e, _ := uploadECU()
	node := e.node()
	node.Fault = gocantest.Drop(1)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := newClient(t, node).TransferData(ctx, 16); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the context's deadline", err)
	}
}

func TestReadRAM(t *testing.T) {
	mem := testImage(0x400)
	e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: mem}
	c, node := newT7(t, e)
	got, err := c.ReadRAM(t.Context(), 0x200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, mem[0x200:0x214]) {
		t.Fatalf("got % 02X, want % 02X", got, mem[0x200:0x214])
	}
	if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x05, READ_MEMORY_BY_ADDRESS, 0x00, 0x02, 0x00, 20}; !bytes.Equal(got, want) {
		t.Fatalf("sent % 02X, want % 02X", got, want)
	}

	c, _ = newT7(t, &fakeT7{mem: mem})
	if _, err := c.ReadRAM(t.Context(), 0x200, 20); !errors.Is(err, ErrSecurityAccessDeniedOrRequested) {
		t.Fatalf("locked ECU: err = %v, want %v", err, ErrSecurityAccessDeniedOrRequested)
	}
}

func TestWriteDataByAddress(t *testing.T) {
	data := testImage(10)
	for name, write := range map[string]func(*Client, context.Context, uint32, []byte) error{
		"WriteDataByAddress": (*Client).WriteDataByAddress,
		"SaveRAM":            (*Client).SaveRAM,
	} {
		t.Run(name, func(t *testing.T) {
			e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: make([]byte, 0x100)}
			c, node := newT7(t, e)
			if err := write(c, t.Context(), 0x20, data); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(e.mem[0x20:0x2A], data) {
				t.Fatalf("ECU memory % 02X, want % 02X", e.mem[0x20:0x2A], data)
			}
			m := e.requests[0]
			if want := append([]byte{WRITE_DATA_BY_ADDRESS, 0x00, 0x00, 0x20, byte(len(data))}, data...); !bytes.Equal(m[1:1+len(want)], want) {
				t.Fatalf("ECU got % 02X, want % 02X after the length byte", m[1:], want)
			}
			rows := sentOn(node, REQ_MSG_ID)
			for i, r := range rows {
				if last := i == len(rows)-1; (r[0]&0x80 == 0) != last {
					t.Fatalf("row %d % 02X: every row but the last asks for a 0x270 confirmation", i, r)
				}
			}
		})
	}
}

func TestWriteDataByAddressRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		access  byte
		addr    uint32
		wantErr error
	}{
		{"locked", NO_PRIORITY, 0x20, ErrSecurityAccessDeniedOrRequested},
		{"out of range", DEVELOPMENT_PRIORITY, 0xFC, ErrCannotDownloadToSpecifiedAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newT7(t, &fakeT7{access: tc.access, mem: make([]byte, 0x100)})
			checkErrIs(t, c.WriteDataByAddress(t.Context(), tc.addr, testImage(10)), tc.wantErr)
		})
	}
}

func TestWriteDataByAddressOtherService(t *testing.T) {
	e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: make([]byte, 0x100), answer: map[byte]func([]byte) []byte{
		WRITE_DATA_BY_ADDRESS: answerWith(WRITE_DATA_BY_LOCAL_IDENTIFIER | 0x40),
	}}
	c, _ := newT7(t, e)
	if err := c.WriteDataByAddress(t.Context(), 0x20, testImage(10)); err == nil {
		t.Fatal("WriteDataByAddress accepted a 7B reply")
	}
}

func TestWriteDataByAddressUnconfirmedRow(t *testing.T) {
	shortTimeout(t)
	e := &fakeT7{access: DEVELOPMENT_PRIORITY, mem: make([]byte, 0x100)}
	node := e.node()
	node.Fault = gocantest.Drop(0)
	if err := newClient(t, node).WriteDataByAddress(t.Context(), 0x20, testImage(10)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if e.mem[0x20] != 0 {
		t.Fatal("ECU memory written")
	}
}
