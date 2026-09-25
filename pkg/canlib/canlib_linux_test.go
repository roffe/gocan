//go:build linux

package canlib

import "testing"

func TestLibcanlib(t *testing.T) {
	if err := Init(); err != nil {
		t.Skip("libcanlib not installed:", err)
	}
	t.Log("CANlib", GetVersion())
	if s, err := GetErrorText(int(ERR_PARAM)); err != nil || s == "" {
		t.Fatalf("GetErrorText(ERR_PARAM) = %q, %v", s, err)
	}
	n, err := GetNumberOfChannels()
	if err != nil {
		t.Fatal(err)
	}
	for ch := range n {
		name, err := GetChannelDataString(ch, CHANNELDATA_DEVDESCR_ASCII)
		t.Logf("channel %d: %q %v", ch, name, err)
	}
}
