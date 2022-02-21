package model

import "time"

type CANFrame interface {
	Identifier() uint32
	Len() int
	Data() []byte
	Type() CANFrameType
	String() string
	SetTimeout(time.Duration)
	GetTimeout() time.Duration
}

type CANFrameType int

const (
	Incoming CANFrameType = iota
	Outgoing
	ResponseRequired
)
