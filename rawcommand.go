package gocan

import "time"

type RawCommand struct {
	data    string
	timeout time.Duration
}

func NewRawCommand(data string) *RawCommand {
	return &RawCommand{data: data}
}

func (r *RawCommand) Identifier() uint32 {
	return 0
}

func (r *RawCommand) Length() int {
	return len(r.data)
}

func (r *RawCommand) Data() []byte {
	return []byte(r.data)
}

func (r *RawCommand) Type() CANFrameType {
	return Outgoing
}

func (r *RawCommand) String() string {
	return r.data
}

func (f *RawCommand) SetTimeout(t time.Duration) {
	f.timeout = t
}

func (f *RawCommand) Timeout() time.Duration {
	return f.timeout
}
