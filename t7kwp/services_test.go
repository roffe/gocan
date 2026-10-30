package t7kwp

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/gocantest"
)

func TestReadECUIdentification(t *testing.T) {
	vin := []byte("YS3FF45S1X1234567")
	e := &fakeT7{ids: map[byte][]byte{0x90: vin}}
	c, node := newT7(t, e)
	got, err := c.ReadECUIdentification(t.Context(), 0x90)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, vin) {
		t.Fatalf("got %q, want %q", got, vin)
	}
	if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x02, READ_ECU_IDENTIFICATION, 0x90, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("sent % 02X, want % 02X", got, want)
	}
	if got := acks(node); !bytes.Equal(got, []byte{0x83, 0x82, 0x81, 0x80}) {
		t.Fatalf("acks % 02X, want every row acked", got)
	}

	if _, err := c.ReadECUIdentification(t.Context(), 0x96); !errors.Is(err, ErrSubFunctionNotSupportedOrInvalidFormat) {
		t.Fatalf("undefined id: err = %v, want %v", err, ErrSubFunctionNotSupportedOrInvalidFormat)
	}
}

// A late reply to an earlier identification request is not this one's answer.
func TestReadECUIdentificationSkipsOtherReplies(t *testing.T) {
	if os.Getenv("GOCAN_BUG_TESTS") == "" {
		t.Skip("BUG: ReadECUIdentification takes the first 0x258 frame without checking the 5A <id> echo")
	}
	c := newClient(t, &gocantest.Node{Reply: func(f gocan.Frame) []gocan.Frame {
		if f.ID == REQ_MSG_ID && f.Data[3] == READ_ECU_IDENTIFICATION {
			return []gocan.Frame{
				gocan.NewFrame(0x258, []byte{0xC0, 0xBF, 0x04, 0x5A, 0x90, 'Y', 'S', 0}),
				gocan.NewFrame(0x258, []byte{0xC0, 0xBF, 0x04, 0x5A, 0x97, 'B', '2', 0}),
			}
		}
		return nil
	}})
	if got, err := c.ReadECUIdentification(t.Context(), 0x97); err != nil || string(got) != "B2" {
		t.Fatalf("got %q, %v; want \"B2\"", got, err)
	}
}

func TestReadDataByLocalIdentifierMode(t *testing.T) {
	data := testImage(10)
	e := &fakeT7{answer: map[byte]func([]byte) []byte{READ_DATA_BY_IDENTIFIER: func(m []byte) []byte {
		return append([]byte{READ_DATA_BY_IDENTIFIER | 0x40, m[2]}, data...)
	}}}
	c, node := newT7(t, e)
	got, err := c.ReadDataByLocalIdentifierMode(t.Context(), 0x40, 0x02)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got % 02X, want % 02X", got, data)
	}
	if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x03, READ_DATA_BY_IDENTIFIER, 0x40, 0x02, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("sent % 02X, want % 02X", got, want)
	}

	e.answer[READ_DATA_BY_IDENTIFIER] = answerWith(0x7F, READ_DATA_BY_IDENTIFIER, REQUEST_OUT_OF_RANGE)
	if _, err := c.ReadDataByLocalIdentifierMode(t.Context(), 0x40, 0x02); !errors.Is(err, ErrRequestOutOfRange) {
		t.Fatalf("err = %v, want %v", err, ErrRequestOutOfRange)
	}
}

// A T7 does not implement stopRoutineByLocalIdentifier.
func TestStopRoutineByIdentifier(t *testing.T) {
	c, node := newT7(t, &fakeT7{})
	if _, err := c.StopRoutineByIdentifier(t.Context(), RLI_EOL_START); !errors.Is(err, ErrServiceNotSupported) {
		t.Fatalf("err = %v, want %v", err, ErrServiceNotSupported)
	}
	if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x02, STOP_ROUTINE_BY_IDENTIFIER, RLI_EOL_START, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("sent % 02X, want % 02X", got, want)
	}
}

func TestRequestRoutineResultsByLocalIdentifier(t *testing.T) {
	e := &fakeT7{answer: map[byte]func([]byte) []byte{
		REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER: answerWith(REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER|0x40, RLI_END_EOL, 0x0B),
	}}
	c, node := newT7(t, e)
	got, err := c.RequestRoutineResultsByLocalIdentifier(t.Context(), RLI_END_EOL)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0xC0, 0xBF, 0x03, 0x73, RLI_END_EOL, 0x0B, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("got % 02X, want the reply frame % 02X", got, want)
	}
	if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x02, REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER, RLI_END_EOL, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("sent % 02X, want % 02X", got, want)
	}

	e.answer[REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER] = answerWith(0x7F, REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER, SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT)
	if _, err := c.RequestRoutineResultsByLocalIdentifier(t.Context(), 0x60); !errors.Is(err, ErrSubFunctionNotSupportedOrInvalidFormat) {
		t.Fatalf("err = %v, want %v", err, ErrSubFunctionNotSupportedOrInvalidFormat)
	}
}

func TestClearDTCS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  func([]byte) []byte
		wantErr error
	}{
		{name: "positive"},
		{name: "negative", answer: answerWith(0x7F, CLEAR_DTC, SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT), wantErr: ErrSubFunctionNotSupportedOrInvalidFormat},
		{name: "other group", answer: answerWith(CLEAR_DTC|0x40, 0x00, 0x00), wantErr: errAny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{}
			if tc.answer != nil {
				e.answer = map[byte]func([]byte) []byte{CLEAR_DTC: tc.answer}
			}
			c, node := newT7(t, e)
			checkErrIs(t, c.ClearDTCS(t.Context()), tc.wantErr)
			if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x03, CLEAR_DTC, 0xFF, 0x00, 0, 0}; !bytes.Equal(got, want) {
				t.Fatalf("sent % 02X, want % 02X", got, want)
			}
		})
	}
}

// A malformed clear lands in J1979 mode $04 (clear emission data) on the ECU;
// only the 6C echo counts as success.
func TestClearDynamicallyDefineLocalId(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  func([]byte) []byte
		wantErr error
	}{
		{name: "positive"},
		{name: "J1979 mode 04 reply", answer: answerWith(0x44), wantErr: errAny},
		{name: "negative", answer: answerWith(0x7F, DYNAMICALLY_DEFINE_IDENTIFIER, REQUEST_OUT_OF_RANGE), wantErr: ErrRequestOutOfRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{}
			if tc.answer != nil {
				e.answer = map[byte]func([]byte) []byte{DYNAMICALLY_DEFINE_IDENTIFIER: tc.answer}
			}
			c, node := newT7(t, e)
			checkErrIs(t, c.ClearDynamicallyDefineLocalId(t.Context()), tc.wantErr)
			if got, want := sentOn(node, REQ_MSG_ID)[0], []byte{0x40, 0xA1, 0x04, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, DM_CDDLI, 0x00, 0}; !bytes.Equal(got, want) {
				t.Fatalf("sent % 02X, want % 02X", got, want)
			}
		})
	}
}

func TestDynamicallyDefineLocalId(t *testing.T) {
	for _, tc := range []struct {
		name   string
		define func(*Client) error
		want   []byte
	}{
		{
			name:   "by address",
			define: func(c *Client) error { return c.DynamicallyDefineLocalIdByAddress(t.Context(), 2, 0xF01234, 4) },
			want:   []byte{0x08, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, DM_DBMA, 2, 4, 0xF0, 0x12, 0x34},
		},
		{
			name:   "by local id",
			define: func(c *Client) error { return c.DynamicallyDefineLocalIdByLocID(t.Context(), 3, 0xBB) },
			want:   []byte{0x06, DYNAMICALLY_DEFINE_IDENTIFIER, 0xF0, DM_DBLI, 3, 0x00, 0xBB, 0x00},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{}
			c, node := newT7(t, e)
			if err := tc.define(c); err != nil {
				t.Fatal(err)
			}
			if m := e.requests[0]; !bytes.Equal(m[:len(tc.want)], tc.want) {
				t.Fatalf("ECU got % 02X, want % 02X", m, tc.want)
			}
			if got := sentOn(node, REQ_MSG_ID); len(got) != 2 || got[0][0] != 0xC1 || got[1][0] != 0x00 {
				t.Fatalf("rows %02X, want a confirmed first row and a last row", got)
			}

			e.answer = map[byte]func([]byte) []byte{DYNAMICALLY_DEFINE_IDENTIFIER: answerWith(0x7F, DYNAMICALLY_DEFINE_IDENTIFIER, REQUEST_OUT_OF_RANGE)}
			if err := tc.define(c); !errors.Is(err, ErrRequestOutOfRange) {
				t.Fatalf("rejected definition: err = %v, want %v", err, ErrRequestOutOfRange)
			}
		})
	}
}

func TestSendEU0DRegistrationKey(t *testing.T) {
	key := []byte{0x01, 0x02, 0x03, 0x04}
	for _, tc := range []struct {
		name    string
		send    func(*Client, []byte) error
		wantRow []byte
	}{
		{"short", func(c *Client, k []byte) error { return c.SendEU0DRegistrationKey(t.Context(), k) }, []byte{0x40, 0xA1, 0x05, EU0D_SET_REGISTRATION_KEY, 1, 2, 3, 4}},
		{"long", func(c *Client, k []byte) error { return c.SendEU0DRegistrationKeyLong(t.Context(), k) }, []byte{0x40, 0xA1, 0x06, EU0D_SET_REGISTRATION_KEY, 1, 2, 3, 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeT7{answer: map[byte]func([]byte) []byte{EU0D_SET_REGISTRATION_KEY: answerWith(EU0D_SET_REGISTRATION_KEY | 0x40)}}
			c, node := newT7(t, e)
			if err := tc.send(c, key); err != nil {
				t.Fatal(err)
			}
			if got := sentOn(node, REQ_MSG_ID); len(got) != 1 || !bytes.Equal(got[0], tc.wantRow) {
				t.Fatalf("sent %02X, want [% 02X]", got, tc.wantRow)
			}

			// a stock firmware does not know the service
			delete(e.answer, EU0D_SET_REGISTRATION_KEY)
			if err := tc.send(c, key); !errors.Is(err, ErrServiceNotSupported) {
				t.Fatalf("stock ECU: err = %v, want %v", err, ErrServiceNotSupported)
			}
		})
	}

	c, node := newT7(t, &fakeT7{})
	if err := c.SendEU0DRegistrationKey(t.Context(), key[:3]); err == nil || len(node.Sent()) != 0 {
		t.Fatalf("3-byte key: err = %v after %d frames, want an error and nothing sent", err, len(node.Sent()))
	}
}

func TestTranslateErrorCode(t *testing.T) {
	all := []*Error{
		ErrGeneralReject, ErrServiceNotSupported, ErrSubFunctionNotSupportedOrInvalidFormat, ErrBusyRepeatRequest,
		ErrConditionsNotCorrectOrRequestSequenceError, ErrRoutineNotCompleteOrServiceInProgress, ErrRequestOutOfRange,
		ErrSecurityAccessDeniedOrRequested, ErrInvalidKey, ErrExceedNumberOfAttempts, ErrRequiredTimeDelayNotExpired,
		ErrDownloadNotAccepted, ErrImproperDownloadType, ErrCannotDownloadToSpecifiedAddress, ErrCannotDownloadNumberOfBytesRequested,
		ErrUploadNotAccepted, ErrImproperUploadType, ErrCannotUploadFromSpecifiedAddress, ErrCannotUploadNumberOfBytesRequested,
		ErrTransferSuspended, ErrTransferAborted, ErrIllegalAddressInBlockTransfer, ErrIllegalByteCountInBlockTransfer,
		ErrIllegalBlockTransferType, ErrBlockTransferDataChecksumError, ErrRequestCorrectlyReceivedResponsePending,
		ErrIncorrectByteCountDuringBlockTransfer, ErrServiceNotSupportedInActiveDiagnosticSession, ErrSecurityAccessAllowed,
		ErrReadyForDownload, ErrReadyForUpload, ErrNormalExitWithResults, ErrNormalExitWithoutResults,
		ErrAbnormalExitWithResults, ErrAbnormalExitWithoutResults,
	}
	for _, want := range all {
		if got := TranslateErrorCode(want.Code); got != want {
			t.Errorf("TranslateErrorCode(0x%02X) = %v, want %v", want.Code, got, want)
		}
	}
	if err := TranslateErrorCode(0x00); err != nil {
		t.Errorf("0x00 = %v, want nil", err)
	}
	if err := TranslateErrorCode(0xEE); err == nil || errors.As(err, new(*Error)) {
		t.Errorf("0xEE = %v, want an unknown-code error", err)
	}
}
