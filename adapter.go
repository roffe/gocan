package gocan

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Adapter interface {
	Name() string
	Open(context.Context) error
	Close() error
	Send() chan<- *CANFrame
	Recv() <-chan *CANFrame
	Err() <-chan error
	Event() <-chan Event
}

type AdapterInfo struct {
	Name               string
	Description        string
	RequiresSerialPort bool
	Capabilities       AdapterCapabilities
	New                func(*AdapterConfig) (Adapter, error)
}

func (a *AdapterInfo) String() string {
	return fmt.Sprintf("%s | %s, requires serial port: %v ", a.Name, a.Description, a.RequiresSerialPort)
}

type AdapterCapabilities struct {
	HSCAN bool
	SWCAN bool
	KLine bool
}

func (a *AdapterCapabilities) String() string {
	return fmt.Sprintf("HSCAN: %v, SWCAN: %v, KLine: %v", a.HSCAN, a.SWCAN, a.KLine)
}

type AdapterConfig struct {
	Debug            bool              // enable debug logging
	Port             string            // port name or path to dll/so/dylib
	PortBaudrate     int               // port baudrate in bps (only used for serial based adapters)
	CANRate          float64           // CAN bus rate in kbit/s
	CANFilter        []uint32          // CAN ID filters
	UseExtendedID    bool              // only used for j2534 when setting upp frame filters
	PrintVersion     bool              // print adapter version info on open
	AdditionalConfig map[string]string // Key value pairs for adapter specific configuration
}

var adapterMap = make(map[string]*AdapterInfo)

func NewAdapter(adapterName string, cfg *AdapterConfig) (Adapter, error) {
	if adapter, found := adapterMap[adapterName]; found {
		return adapter.New(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

func RegisterAdapter(adapter *AdapterInfo) error {
	if _, found := adapterMap[adapter.Name]; !found {
		adapterMap[adapter.Name] = adapter
		return nil
	}
	return fmt.Errorf("adapter %s already registered", adapter.Name)
}

func ListAdapterNames() []string {
	var out []string
	for name := range adapterMap {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func ListAdapters() []AdapterInfo {
	var out []AdapterInfo
	for _, adapter := range adapterMap {
		out = append(out, *adapter)
	}
	return out
}
