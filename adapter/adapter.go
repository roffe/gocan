package adapter

import (
	"fmt"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/j2534"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
)

const (
	Canusb AdapterID = iota
	OBDLinkSX
	Mangoose
)

type AdapterID int

type NewAdapterFunc func(*gocan.AdapterConfig) (gocan.Adapter, error)

type AdapterItem struct {
	ID    AdapterID
	New   NewAdapterFunc
	Name  string
	Alias []string
}

var adapterList = []AdapterItem{
	{
		ID:   Canusb,
		New:  lawicel.NewCanusb,
		Name: "Canusb",
	},
	{
		ID:    OBDLinkSX,
		New:   obdlink.NewSX,
		Name:  "OBDLink SX",
		Alias: []string{"obdlinksx", "sx"},
	},
	{
		ID:   Mangoose,
		New:  j2534.NewMangoose,
		Name: "Mangoose",
	},
}

func ListAdapters() []AdapterItem {
	return adapterList
}

func ListAdapterStrings() []string {
	var out []string
	for _, a := range adapterList {
		out = append(out, a.Name)
	}
	return out
}

func New(adapter interface{}, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	switch t := adapter.(type) {
	case string:
		normalized := strings.ToLower(t)
		for _, a := range adapterList {
			if strings.ToLower(a.Name) == normalized {
				return a.New(cfg)
			}
			for _, alias := range a.Alias {
				if normalized == strings.ToLower(alias) {
					return a.New(cfg)
				}
			}
		}
	case int, AdapterID:
		for _, a := range adapterList {
			if t == a.ID {
				return a.New(cfg)
			}
		}
	default:
		return nil, fmt.Errorf("invalid type %t", t)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapter)
}
