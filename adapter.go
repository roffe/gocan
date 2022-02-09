package gocan

import "context"

type Adapter interface {
	Init() error
	SetPort(string) error
	SetPortRate(int) error
	SetCANrate(float64) error
	SetCANfilter(...uint32)
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
}
