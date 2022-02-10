package gocan

import "github.com/roffe/gocan/pkg/model"

type Adapter interface {
	Init() error
	SetPort(string) error
	SetPortRate(int) error
	SetCANrate(float64) error
	SetCANfilter(...uint32)
	Chan() <-chan model.CANFrame
	Send(model.CANFrame) error
	Close() error
}
