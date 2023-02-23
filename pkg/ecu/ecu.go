package ecu

import (
	"context"
	"errors"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/ecu/t5"
	"github.com/roffe/gocan/pkg/ecu/t7"
	"github.com/roffe/gocan/pkg/ecu/t8"
	"github.com/roffe/gocan/pkg/ecu/t8mcp"
	"github.com/roffe/gocan/pkg/model"
)

const (
	UnknownECU Type = -1
	Trionic5   Type = iota
	Trionic7
	Trionic8
	Trionic8MCP
	Me96
)

type Client interface {
	ReadDTC(context.Context) ([]model.DTC, error)
	PrintECUInfo(context.Context) error
	Info(context.Context, model.ProgressCallback) ([]model.HeaderResult, error)
	DumpECU(context.Context, model.ProgressCallback) ([]byte, error)
	FlashECU(context.Context, []byte, model.ProgressCallback) error
	EraseECU(context.Context, model.ProgressCallback) error
	ResetECU(context.Context, model.ProgressCallback) error
}

func FromString(s string) Type {
	switch strings.ToLower(s) {
	case "5", "t5", "trionic5", "trionic 5":
		return Trionic5
	case "7", "t7", "trionic7", "trionic 7":
		return Trionic7
	case "8", "t8", "trionic8", "trionic 8":
		return Trionic8
	case "mcp", "t8mcp", "trionic8mcp", "trionic 8 mcp":
		return Trionic8MCP
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
	case Trionic8MCP:
		return "Trionic 8 MCP"
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
	case Trionic8MCP:
		return t8mcp.New(c), nil
	default:
		return nil, errors.New("unknown ECU")
	}
}

func CANFilters(t Type) []uint32 {
	switch t {
	case Trionic5:
		return []uint32{0x0, 0x05, 0x06, 0x0C}
	case Trionic7:
		return []uint32{0x220, 0x238, 0x240, 0x258, 0x266}
	case Trionic8:
		return []uint32{0x5E8, 0x7E0, 0x7E8}
	case Trionic8MCP:
		return []uint32{0x7E0, 0x7E8}
	default:
		return []uint32{}
	}
}
