package model

import "time"

type RawCommand struct {
	data string
}

func NewRawCommand(data string) *RawCommand {
	return &RawCommand{data: data}
}

func (r *RawCommand) Identifier() uint32 {
	return 0
}

func (r *RawCommand) Len() int {
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
	// noop for interface
}

func (f *RawCommand) GetTimeout() time.Duration {
	return time.Duration(0)
}
