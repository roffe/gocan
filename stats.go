package canusb

import "fmt"

type Stats struct {
	RecvBytes     uint64
	SentBytes     uint64
	Errors        uint64
	DroppedFrames uint64
}

func (st *Stats) String() string {
	return fmt.Sprintf("recv: %d sent: %d errors: %d dropped : %d\n", st.RecvBytes, st.SentBytes, st.Errors, st.DroppedFrames)
}

func (c *Canusb) Stats() Stats {
	return Stats{c.recvBytes, c.sentBytes, c.errors, c.dropped}
}
