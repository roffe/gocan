//go:build pcan

package pcan

import (
	"testing"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/pcan"
)

func TestMsgConversion(t *testing.T) {
	for _, f := range []gocan.Frame{
		gocan.NewFrame(0x240, []byte{0x40, 0xA1, 0x02, 0x21, 0xF0}),
		gocan.NewExtendedFrame(0x18DAF110, []byte{1, 2, 3}),
		{ID: 0x7E0, Remote: true, Length: 2},
	} {
		msg := toMsg(f)
		if got := msg.MSGTYPE&pcan.PCAN_MESSAGE_EXTENDED != 0; got != f.Extended {
			t.Errorf("%+v: extended flag %v", f, got)
		}
		if got, ok := fromMsg(&msg); !ok || got != f {
			t.Errorf("round trip %+v -> %+v (%v)", f, got, ok)
		}
	}
	status := pcan.TPCANMsg{MSGTYPE: pcan.PCAN_MESSAGE_STATUS, LEN: 4}
	if _, ok := fromMsg(&status); ok {
		t.Error("status message delivered as a frame")
	}
}
