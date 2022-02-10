package model

type CANFrame interface {
	GetIdentifier() uint32
	Byte() []byte
	GetData() []byte
	String() string
}
