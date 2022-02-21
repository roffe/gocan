package gocan

import (
	"context"

	"github.com/roffe/gocan/pkg/model"
)

type Adapter interface {
	Init(context.Context) error
	SetPort(string) error
	SetPortRate(int) error
	SetCANrate(float64) error
	SetCANfilter(...uint32)
	Chan() <-chan model.CANFrame
	Send(model.CANFrame) error
	Close() error
}

type Trionic interface {
	Info(context.Context) ([]model.HeaderResult, error)
}
