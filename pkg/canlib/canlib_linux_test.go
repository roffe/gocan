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

// virtualPair opens two Kvaser virtual channels, which loop back to each
// other, so the bindings can be exercised without a real bus.
func virtualPair(tb testing.TB) (tx, rx Handle) {
	if err := Init(); err != nil {
		tb.Skip("libcanlib not installed:", err)
	}
	n, _ := GetNumberOfChannels()
	var virt []int
	for ch := range n {
		if s, _ := GetChannelDataString(ch, CHANNELDATA_DEVDESCR_ASCII); s == "Kvaser Virtual CAN" {
			virt = append(virt, ch)
		}
	}
	if len(virt) < 2 {
		tb.Skip("need two Kvaser virtual channels")
	}
	for i, h := range []*Handle{&tx, &rx} {
		var err error
		if *h, err = OpenChannel(virt[i], OPEN_ACCEPT_VIRTUAL|OPEN_REQUIRE_INIT_ACCESS); err != nil {
			tb.Fatal(err)
		}
		if err := h.SetBusParams(BITRATE_500K, 0, 0, 0, 0, 0); err != nil {
			tb.Fatal(err)
		}
		if err := h.BusOn(); err != nil {
			tb.Fatal(err)
		}
		tb.Cleanup(func() { h.BusOff(); h.Close() })
	}
	return tx, rx
}

// TestHotPathAllocs guards the per-frame calls: the only allocation left
// is SyscallN's variadic argument slice.
func TestHotPathAllocs(t *testing.T) {
	tx, rx := virtualPair(t)
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	msg := new(CANMessage)
	n := testing.AllocsPerRun(100, func() {
		if err := tx.Write(0x1FFFFFFF, data, MSG_EXT); err != nil {
			t.Fatal(err)
		}
		if err := rx.ReadWait(msg, 100); err != nil {
			t.Fatal(err)
		}
	})
	if n > 2 {
		t.Errorf("Write+ReadWait allocates %v times, want <= 2", n)
	}
	if msg.Identifier != 0x1FFFFFFF || MsgFlag(msg.Flags)&MSG_EXT == 0 || msg.DLC != 8 || msg.Data[7] != 8 {
		t.Errorf("got %+v", msg)
	}
}

func BenchmarkWriteRead(b *testing.B) {
	tx, rx := virtualPair(b)
	data := []byte{0x40, 0xA1, 0x02, 0x1A, 0x90, 0, 0, 0}
	msg := new(CANMessage)
	b.ReportAllocs()
	for b.Loop() {
		if err := tx.Write(0x240, data, MSG_STD); err != nil {
			b.Fatal(err)
		}
		if err := rx.ReadWait(msg, 100); err != nil {
			b.Fatal(err)
		}
		if msg.Identifier != 0x240 || msg.DLC != 8 || msg.Data[1] != 0xA1 {
			b.Fatalf("got %+v", msg)
		}
	}
}

func BenchmarkReadWaitTimeout(b *testing.B) {
	_, rx := virtualPair(b)
	msg := new(CANMessage)
	b.ReportAllocs()
	for b.Loop() {
		if err := rx.ReadWait(msg, 0); err != ErrNoMsg && err != ErrTimeout {
			b.Fatal(err)
		}
	}
}
