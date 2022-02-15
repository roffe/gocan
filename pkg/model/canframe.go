package model

type CANFrame interface {
	Identifier() uint32
	Len() int
	Data() []byte
	Type() CANFrameType
	String() string
}

type CANFrameType int

const (
	Incoming CANFrameType = iota
	Outgoing
	OutResponseRequired
)
