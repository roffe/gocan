package gocan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Adapter is a hardware (or virtual) CAN interface backend. The Client owns
// the adapter: it drains Recv, Err and Event and feeds Send. Implementations
// deliver incoming frames on Recv, report recoverable problems on Event and
// signal an unrecoverable failure on Err, which terminates the client.
type Adapter interface {
	Name() string
	Open(context.Context) error
	Close() error
	Send() chan<- *CANFrame
	Recv() <-chan *CANFrame
	Err() <-chan error
	Event() <-chan Event
}

// AdapterInfo describes a registered adapter: its registry name, what it
// needs (serial port) and what it can do.
type AdapterInfo struct {
	Name               string
	Description        string
	RequiresSerialPort bool
	SerialPortOptional bool
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

// AdapterConfig holds settings applied when an adapter is opened. Not every
// field applies to every adapter; the comments note the main consumers.
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

var (
	adapterMu  sync.Mutex
	adapterMap = make(map[string]*AdapterInfo)
)

// NewAdapter constructs a registered adapter by name without opening it. Use
// ListAdapterNames to discover what is available in this build.
func NewAdapter(adapterName string, cfg *AdapterConfig) (Adapter, error) {
	adapterMu.Lock()
	adapter, found := adapterMap[adapterName]
	adapterMu.Unlock()
	if found {
		return adapter.New(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

// RegisterAdapter adds an adapter to the registry, typically from an init
// function. It fails if the name is already taken.
func RegisterAdapter(adapter *AdapterInfo) error {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	if _, found := adapterMap[adapter.Name]; !found {
		adapterMap[adapter.Name] = adapter
		return nil
	}
	return fmt.Errorf("adapter %s already registered", adapter.Name)
}

// ListAdapterNames returns the names of all registered adapters, sorted
// case-insensitively.
func ListAdapterNames() []string {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	out := make([]string, 0, len(adapterMap))
	for name := range adapterMap {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// ListAdapters returns a copy of every registered adapter's info.
func ListAdapters() []AdapterInfo {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	out := make([]AdapterInfo, 0, len(adapterMap))
	for _, adapter := range adapterMap {
		out = append(out, *adapter)
	}
	return out
}
