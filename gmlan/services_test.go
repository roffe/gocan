package gmlan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

// service is one GMLAN exchange as the ECU sees it: req is the request
// payload it must receive, resp the positive response payload it answers.
type service struct {
	name string
	call func(context.Context, *Client) error
	req  []byte
	resp []byte
}

var (
	vin       = []byte("YS3FH41U281234567")
	ramData   = []byte{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF}
	ramWrite  = append([]byte{WRITE_DATA_BY_IDENTIFIER, 0x15, 0x10, 0x20, 0x30, byte(len(ramData))}, ramData...)
	pinCode   = []byte{0x00, '1', '2', '3', '4'}
	vinWrite  = append([]byte{WRITE_DATA_BY_IDENTIFIER, 0x90}, vin...)
	initDiag  = []byte{INITIATE_DIAGNOSTIC_OPERATION, byte(LEV_DADTC)}
	silentECU = func([]byte) [][]byte { return nil }
)

// expect fails a call that returned the wrong value with a nil error.
func expect[T any](got T, err error, want T) error {
	if err == nil && !reflect.DeepEqual(got, want) {
		return fmt.Errorf("got %v, want %v", got, want)
	}
	return err
}

var services = []service{
	{
		name: "ClearDiagnosticInformation",
		call: func(ctx context.Context, cl *Client) error { return cl.ClearDiagnosticInformation(ctx, 0x7E0) },
		req:  []byte{0x04},
		resp: []byte{0x44},
	},
	{
		name: "InitiateDiagnosticOperation",
		call: func(ctx context.Context, cl *Client) error { return cl.InitiateDiagnosticOperation(ctx, LEV_DADTC) },
		req:  initDiag,
		resp: []byte{0x50},
	},
	{
		name: "ReturnToNormalMode",
		call: func(ctx context.Context, cl *Client) error { return cl.ReturnToNormalMode(ctx) },
		req:  []byte{RETURN_TO_NORMAL_MODE},
		resp: []byte{0x60},
	},
	{
		name: "DisableNormalCommunication",
		call: func(ctx context.Context, cl *Client) error { return cl.DisableNormalCommunication(ctx) },
		req:  []byte{DISABLE_NORMAL_COMMUNICATION},
		resp: []byte{0x68},
	},
	{
		name: "DynamicallyDefineMessage",
		call: func(ctx context.Context, cl *Client) error { return cl.DynamicallyDefineMessage(ctx, 0x038D, 0x0101) },
		req:  []byte{DYNAMICALLY_DEFINE_MESSAGE, 0xFE, 0x03, 0x8D, 0x01, 0x01},
		resp: []byte{0x6C, 0xFE},
	},
	{
		name: "ReportProgrammedState",
		call: func(ctx context.Context, cl *Client) error {
			state, err := cl.ReportProgrammedState(ctx)
			return expect(state, err, 0x02)
		},
		req:  []byte{REPORT_PROGRAMMED_STATE},
		resp: []byte{0xE2, 0x02},
	},
	{
		name: "SecurityAccessRequestSeed",
		call: func(ctx context.Context, cl *Client) error {
			seed, err := cl.SecurityAccessRequestSeed(ctx, AccessLevelFD)
			return expect(seed, err, []byte{0x12, 0x34})
		},
		req:  []byte{SECURITY_ACCESS, AccessLevelFD},
		resp: []byte{0x67, AccessLevelFD, 0x12, 0x34},
	},
	{
		name: "SecurityAccessSendKey",
		call: func(ctx context.Context, cl *Client) error {
			return cl.SecurityAccessSendKey(ctx, AccessLevelFD, 0xAB, 0xCD)
		},
		req:  []byte{SECURITY_ACCESS, AccessLevelFD + 1, 0xAB, 0xCD},
		resp: []byte{0x67, AccessLevelFD + 1},
	},
	{
		name: "RequestDownload",
		call: func(ctx context.Context, cl *Client) error { return cl.RequestDownload(ctx, false) },
		req:  []byte{REQUEST_DOWNLOAD, 0x00, 0x00, 0x00, 0x00, 0x00},
		resp: []byte{0x74},
	},
	{
		name: "RequestDownload z22se",
		call: func(ctx context.Context, cl *Client) error { return cl.RequestDownload(ctx, true) },
		req:  []byte{REQUEST_DOWNLOAD, 0x00, 0x00, 0x00, 0x00},
		resp: []byte{0x74},
	},
	{
		name: "Execute",
		call: func(ctx context.Context, cl *Client) error { return cl.Execute(ctx, 0x102400) },
		req:  []byte{TRANSFER_DATA, 0x80, 0x00, 0x10, 0x24, 0x00},
		resp: []byte{0x76},
	},
	{
		name: "WriteDataByIdentifier empty",
		call: func(ctx context.Context, cl *Client) error { return cl.WriteDataByIdentifier(ctx, 0x63, nil) },
		req:  []byte{WRITE_DATA_BY_IDENTIFIER, 0x63},
		resp: []byte{0x7B, 0x63},
	},
	{
		name: "WriteDataByIdentifierUint16",
		call: func(ctx context.Context, cl *Client) error { return cl.WriteDataByIdentifierUint16(ctx, 0x02, 0x00FA) },
		req:  []byte{WRITE_DATA_BY_IDENTIFIER, 0x02, 0x00, 0xFA},
		resp: []byte{0x7B, 0x02},
	},
	{
		name: "WriteDataByIdentifierUint32",
		call: func(ctx context.Context, cl *Client) error {
			return cl.WriteDataByIdentifierUint32(ctx, 0x25, 0x01020304)
		},
		req:  []byte{WRITE_DATA_BY_IDENTIFIER, 0x25, 0x01, 0x02, 0x03, 0x04},
		resp: []byte{0x7B, 0x25},
	},
	{
		name: "WriteDataByIdentifier multi-frame",
		call: func(ctx context.Context, cl *Client) error { return cl.WriteDataByIdentifier(ctx, 0x90, vin) },
		req:  vinWrite,
		resp: []byte{0x7B, 0x90},
	},
	{
		name: "WriteDataByAddress",
		call: func(ctx context.Context, cl *Client) error { return cl.WriteDataByAddress(ctx, 0x102030, ramData) },
		req:  ramWrite,
		resp: []byte{0x7B, 0x15},
	},
	{
		name: "WriteDataByAddress2",
		call: func(ctx context.Context, cl *Client) error { return cl.WriteDataByAddress2(ctx, 0x102030, ramData) },
		req:  ramWrite,
		resp: []byte{0x7B, 0x15},
	},
	{
		name: "ReadDataByIdentifierUint16",
		call: func(ctx context.Context, cl *Client) error {
			v, err := cl.ReadDataByIdentifierUint16(ctx, 0x02)
			return expect(v, err, 0x00FA)
		},
		req:  []byte{READ_DATA_BY_IDENTIFIER, 0x02},
		resp: []byte{0x5A, 0x02, 0x00, 0xFA},
	},
	{
		name: "ReadDataByIdentifierString",
		call: func(ctx context.Context, cl *Client) error {
			s, err := cl.ReadDataByIdentifierString(ctx, 0x90)
			return expect(s, err, "YS3")
		},
		req:  []byte{READ_DATA_BY_IDENTIFIER, 0x90},
		resp: []byte{0x5A, 0x90, 'Y', 'S', '3', 0x00, 0x00},
	},
	{
		name: "DeviceControl",
		call: func(ctx context.Context, cl *Client) error { return cl.DeviceControl(ctx, 0x16) },
		req:  []byte{DEVICE_CONTROL, 0x16},
		resp: []byte{0xEE, 0x16},
	},
	{
		name: "DeviceControlWithCode",
		call: func(ctx context.Context, cl *Client) error { return cl.DeviceControlWithCode(ctx, 0x60, pinCode) },
		req:  append([]byte{DEVICE_CONTROL, 0x60}, pinCode...),
		resp: []byte{0xEE, 0x60},
	},
}

func TestServices(t *testing.T) {
	for _, s := range services {
		t.Run(s.name, func(t *testing.T) {
			e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{s.resp} }}
			cl, _ := newECUClient(t, e)
			if err := s.call(context.Background(), cl); err != nil {
				t.Fatal(err)
			}
			e.check(t, s.req)
		})
	}
}

func TestServicesNegativeResponse(t *testing.T) {
	const conditionsNotCorrect = 0x22
	for _, s := range services {
		t.Run(s.name, func(t *testing.T) {
			e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{negative(req[0], conditionsNotCorrect)} }}
			cl, _ := newECUClient(t, e)
			wantNRC(t, s.call(context.Background(), cl), s.req[0], conditionsNotCorrect)
			e.check(t, s.req)
		})
	}
}

func TestServicesTimeout(t *testing.T) {
	t.Parallel()
	for _, s := range services {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			e := &fakeECU{handle: silentECU}
			cl, _ := newECUClient(t, e)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if err := s.call(ctx, cl); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("silent ECU: err = %v, want a timeout", err)
			}
			e.check(t, s.req)
		})
	}
}

func TestBusyRepeatRequestIsRetried(t *testing.T) {
	t.Parallel()
	replies := [][]byte{
		negative(INITIATE_DIAGNOSTIC_OPERATION, 0x21),
		{0x60}, // the T8's bare busy frame
		{0x50},
	}
	e := &fakeECU{handle: func([]byte) [][]byte {
		r := replies[0]
		replies = replies[1:]
		return [][]byte{r}
	}}
	cl, _ := newECUClient(t, e)
	if err := cl.InitiateDiagnosticOperation(context.Background(), LEV_DADTC); err != nil {
		t.Fatal(err)
	}
	e.check(t, initDiag, initDiag, initDiag)
}

func TestBusyRepeatRequestGivesUp(t *testing.T) {
	t.Parallel()
	e := &fakeECU{handle: func(req []byte) [][]byte { return [][]byte{negative(req[0], 0x21)} }}
	cl, _ := newECUClient(t, e)
	wantNRC(t, cl.InitiateDiagnosticOperation(context.Background(), LEV_DADTC), INITIATE_DIAGNOSTIC_OPERATION, 0x21)
	e.check(t, slices.Repeat([][]byte{initDiag}, 1+busyRetries)...)
}

func TestBusyRetryWaitEndsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &fakeECU{handle: func(req []byte) [][]byte {
		time.AfterFunc(10*time.Millisecond, cancel)
		return [][]byte{negative(req[0], 0x21)}
	}}
	cl, _ := newECUClient(t, e)
	start := time.Now()
	if err := cl.InitiateDiagnosticOperation(ctx, LEV_DADTC); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed >= busyRetryDelay {
		t.Errorf("returned after %v, want before the %v retry wait is over", elapsed, busyRetryDelay)
	}
	e.check(t, initDiag)
}

func TestResponsePendingThenAnswer(t *testing.T) {
	req := []byte{WRITE_DATA_BY_IDENTIFIER, 0x01, 0x12, 0x34}
	write := func(t *testing.T, final []byte) error {
		e := &fakeECU{handle: func([]byte) [][]byte {
			return [][]byte{negative(WRITE_DATA_BY_IDENTIFIER, 0x78), final}
		}}
		cl, _ := newECUClient(t, e)
		err := cl.WriteDataByIdentifierUint16(context.Background(), 0x01, 0x1234)
		e.check(t, req)
		return err
	}
	t.Run("positive", func(t *testing.T) {
		if err := write(t, []byte{0x7B, 0x01}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("negative", func(t *testing.T) {
		wantNRC(t, write(t, negative(WRITE_DATA_BY_IDENTIFIER, 0x31)), WRITE_DATA_BY_IDENTIFIER, 0x31)
	})
}

func TestResponsePendingExtendsTheWait(t *testing.T) {
	t.Parallel()
	for _, s := range services {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			var node *gocantest.Node
			e := &fakeECU{handle: func(req []byte) [][]byte {
				later(node, 150*time.Millisecond, s.resp...)
				return [][]byte{negative(req[0], 0x78)}
			}}
			node = e.node()
			cl := newTestClient(t, node)
			if err := s.call(context.Background(), cl); err != nil {
				t.Fatalf("final answer 150 ms after $78: %v", err)
			}
			e.check(t, s.req)
		})
	}
}

func TestClearDiagnosticInformationResponsePending(t *testing.T) {
	t.Parallel()
	clear := func(t *testing.T, ctx context.Context, final []byte) error {
		var node *gocantest.Node
		e := &fakeECU{handle: func([]byte) [][]byte {
			if final != nil {
				later(node, 20*time.Millisecond, final...)
			}
			return [][]byte{negative(0x04, 0x78)}
		}}
		node = e.node()
		cl := newTestClient(t, node)
		err := cl.ClearDiagnosticInformation(ctx, 0x7DF)
		if sent := node.Sent(); len(sent) != 1 || sent[0].ID != 0x7DF {
			t.Errorf("sent %v, want one request on 0x7DF", sent)
		}
		e.check(t, []byte{0x04})
		return err
	}
	t.Run("positive", func(t *testing.T) {
		if err := clear(t, context.Background(), []byte{0x44}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("negative", func(t *testing.T) {
		wantNRC(t, clear(t, context.Background(), negative(0x04, 0x22)), 0x04, 0x22)
	})
	t.Run("no final answer", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := clear(t, ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want a timeout", err)
		}
	})
}

// GMW3110 7.2.8: the ECU may repeat $78 until it sends the final response.
func TestClearDiagnosticInformationRepeatedResponsePending(t *testing.T) {
	var node *gocantest.Node
	e := &fakeECU{handle: func([]byte) [][]byte {
		later(node, 20*time.Millisecond, negative(0x04, 0x78)...)
		later(node, 40*time.Millisecond, 0x44)
		return [][]byte{negative(0x04, 0x78)}
	}}
	node = e.node()
	cl := newTestClient(t, node)
	if err := cl.ClearDiagnosticInformation(context.Background(), 0x7DF); err != nil {
		t.Fatal(err)
	}
}

// A buffered adapter hands over the $78 and the final answer in one burst.
func TestClearDiagnosticInformationPendingAndAnswerBackToBack(t *testing.T) {
	e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{negative(0x04, 0x78), {0x44}} }}
	cl, _ := newECUClient(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := cl.ClearDiagnosticInformation(ctx, 0x7DF); err != nil {
		t.Fatal(err)
	}
}

func TestClearDiagnosticInformationIgnoresStaleFrames(t *testing.T) {
	e := &fakeECU{handle: func([]byte) [][]byte {
		return [][]byte{{0x5A, 0x18, 0xAA, 0xBB, 0xCC}, negative(0x04, 0x22)}
	}}
	cl, _ := newECUClient(t, e)
	wantNRC(t, cl.ClearDiagnosticInformation(context.Background(), 0x7DF), 0x04, 0x22)
}

func TestTesterPresentResponseRequired(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{{0x7E}} }}
		cl, _ := newECUClient(t, e)
		if err := cl.TesterPresentResponseRequired(context.Background()); err != nil {
			t.Fatal(err)
		}
		e.check(t, []byte{0x3E})
	})
	t.Run("negative", func(t *testing.T) {
		e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{negative(0x3E, 0x12)} }}
		cl, _ := newECUClient(t, e)
		wantNRC(t, cl.TesterPresentResponseRequired(context.Background()), 0x3E, 0x12)
	})
}

func TestTesterPresentResponseRequiredTimeout(t *testing.T) {
	cl, _ := newECUClient(t, &fakeECU{handle: silentECU})
	if err := cl.TesterPresentResponseRequired(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("silent ECU: err = %v, want a timeout", err)
	}
}

func TestFunctionalRequestsToAllNodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		send func(*Client) error
		want []byte
	}{
		{"TesterPresentNoResponseAllowed", (*Client).TesterPresentNoResponseAllowed, []byte{0xFE, 0x01, 0x3E}},
		{"DisableNormalCommunicationAllNodes", (*Client).DisableNormalCommunicationAllNodes, []byte{0xFE, 0x01, DISABLE_NORMAL_COMMUNICATION}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &gocantest.Node{}
			if err := tc.send(newTestClient(t, node)); err != nil {
				t.Fatal(err)
			}
			if sent := node.Sent(); len(sent) != 1 || sent[0].ID != 0x101 || !bytes.Equal(sent[0].Bytes(), tc.want) {
				t.Errorf("sent %v, want 0x101 [% 02X]", sent, tc.want)
			}
		})
	}
}

func TestReadDataByIdentifierUint16ShortReply(t *testing.T) {
	e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{{0x5A, 0x02, 0x12}} }}
	cl, _ := newECUClient(t, e)
	if v, err := cl.ReadDataByIdentifierUint16(context.Background(), 0x02); err == nil {
		t.Fatalf("one data byte read as %#04x", v)
	}
}

// newPacketClient talks to an ECU that answers a one-shot request for DPID
// $7A with answer.
func newPacketClient(t *testing.T, answer gocan.Frame) (*Client, *gocantest.Node) {
	t.Helper()
	node := &gocantest.Node{Reply: func(req gocan.Frame) []gocan.Frame {
		if !bytes.Equal(req.Bytes(), []byte{0x03, READ_DATA_BY_PACKET_IDENTIFIER, 0x01, 0x7A}) {
			return nil
		}
		return []gocan.Frame{answer}
	}}
	return New(gocantest.Open(t, node), 0x7E0, 0x5E8, 0x7E8), node
}

// uudt is a data packet on 0x5E8: the DPID, then its data.
func uudt(packet []byte) gocan.Frame { return gocan.NewFrame(0x5E8, packet) }

func TestReadDataByPacketIdentifier(t *testing.T) {
	packet := []byte{0x7A, 0x00, 0x52, 0x00, 0x00, 0x00, 0xDC, 0x00}
	cl, node := newPacketClient(t, uudt(packet))
	got, err := cl.ReadDataByPacketIdentifier(context.Background(), 0x01, 0x7A)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, packet) {
		t.Errorf("packet = % 02X, want % 02X", got, packet)
	}
	if sent := node.Sent(); len(sent) != 1 || sent[0].ID != 0x7E0 {
		t.Errorf("sent %v, want one request on 0x7E0", sent)
	}
}

func TestReadDataByPacketIdentifierErrors(t *testing.T) {
	t.Run("negative", func(t *testing.T) {
		cl, _ := newPacketClient(t, sf(negative(READ_DATA_BY_PACKET_IDENTIFIER, 0x31)...))
		_, err := cl.ReadDataByPacketIdentifier(context.Background(), 0x01, 0x7A)
		wantNRC(t, err, READ_DATA_BY_PACKET_IDENTIFIER, 0x31)
	})
	t.Run("silent", func(t *testing.T) {
		cl := New(gocantest.Open(t, &gocantest.Node{}), 0x7E0, 0x5E8, 0x7E8)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if _, err := cl.ReadDataByPacketIdentifier(ctx, 0x01, 0x7A); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want a timeout", err)
		}
	})
}

func TestReadDataByPacketIdentifierDataByte7F(t *testing.T) {
	packet := []byte{0x7A, 0x7F, 0x52, 0x00, 0x00, 0x00, 0xDC, 0x00}
	cl, _ := newPacketClient(t, uudt(packet))
	got, err := cl.ReadDataByPacketIdentifier(context.Background(), 0x01, 0x7A)
	if err != nil || !bytes.Equal(got, packet) {
		t.Fatalf("ReadDataByPacketIdentifier = [% 02X], %v; want [% 02X]", got, err, packet)
	}
}

func TestDynamicallyDefineMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		pids []uint16
		req  []byte
	}{
		{"one frame", []uint16{0x1234, 0x5678}, []byte{DYNAMICALLY_DEFINE_MESSAGE, 0xFE, 0x12, 0x34, 0x56, 0x78}},
		{"multi-frame", []uint16{0x1234, 0x5678, 0x9ABC, 0xDEF0}, []byte{DYNAMICALLY_DEFINE_MESSAGE, 0xFE, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeECU{handle: func([]byte) [][]byte { return [][]byte{{0x6C, 0xFE}} }}
			cl, _ := newECUClient(t, e)
			if err := cl.DynamicallyDefineMessage(context.Background(), tc.pids...); err != nil {
				t.Fatal(err)
			}
			e.check(t, tc.req)
		})
	}
	t.Run("no PIDs", func(t *testing.T) {
		cl, node := newECUClient(t, &fakeECU{handle: silentECU})
		if err := cl.DynamicallyDefineMessage(context.Background()); err == nil {
			t.Fatal("empty definition accepted")
		}
		if sent := node.Sent(); len(sent) != 0 {
			t.Errorf("sent %v", sent)
		}
	})
}

func TestDeviceControlWithCodeTooLongForASingleFrame(t *testing.T) {
	cl, node := newECUClient(t, &fakeECU{handle: silentECU})
	if err := cl.DeviceControlWithCode(context.Background(), 0x60, []byte{1, 2, 3, 4, 5, 6}); err == nil {
		t.Fatal("six control bytes accepted")
	}
	if sent := node.Sent(); len(sent) != 0 {
		t.Errorf("sent %v", sent)
	}
}

func TestResponsePendingLimit(t *testing.T) {
	answer := func(t *testing.T, pending int) error {
		e := &fakeECU{handle: func(req []byte) [][]byte {
			return append(slices.Repeat([][]byte{negative(req[0], 0x78)}, pending), []byte{0x50})
		}}
		cl, _ := newECUClient(t, e)
		return cl.InitiateDiagnosticOperation(context.Background(), LEV_DADTC)
	}
	t.Run("at the limit", func(t *testing.T) {
		if err := answer(t, maxResponsePending); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("past it", func(t *testing.T) {
		wantNRC(t, answer(t, maxResponsePending+1), INITIATE_DIAGNOSTIC_OPERATION, 0x78)
	})
}
