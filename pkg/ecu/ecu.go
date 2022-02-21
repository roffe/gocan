package ecu

import (
	"context"
	"errors"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
	"github.com/roffe/gocan/pkg/t5"
	"github.com/roffe/gocan/pkg/t7"
	"github.com/roffe/gocan/pkg/t8"
)

const (
	UnknownECU Type = -1
	Trionic5   Type = iota
	Trionic7
	Trionic8
	Me96
)

type Client interface {
	Info(context.Context) ([]model.HeaderResult, error)
	PrintECUInfo(context.Context) error
	ResetECU(context.Context) error
	DumpECU(context.Context) ([]byte, error)
}

func TypeFromString(s string) Type {
	switch strings.ToLower(s) {
	case "5", "t5", "trionic5", "trionic 5":
		return Trionic5
	case "7", "t7", "trionic7", "trionic 7":
		return Trionic7
	case "8", "t8", "trionic8", "trionic 8":
		return Trionic8
	case "96", "me9.6", "me96", "me 9.6":
		return Me96
	default:
		return UnknownECU
	}
}

type Type int

func (e Type) String() string {
	switch e {
	case Trionic5:
		return "Trionic 5"
	case Trionic7:
		return "Trionic 7"
	case Trionic8:
		return "Trionic 8"
	case Me96:
		return "Me 9.6"
	default:
		return "Unknown ECU"
	}
}

func New(c *gocan.Client, t Type) (Client, error) {
	switch t {
	case Trionic5:
		return t5.New(c), nil
	case Trionic7:
		return t7.New(c), nil
	case Trionic8:
		return t8.New(c), nil
	default:
		return nil, errors.New("unknown ECU")
	}
}
