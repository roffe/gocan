package gocan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Adapter is a hardware (or virtual) CAN interface backend.
//
// The Bus drives the adapter: it calls Open once, serializes calls to Send
// and calls Close on shutdown. The adapter pushes traffic back through the
// Bus: incoming frames via Bus.Deliver, notifications via Bus.Emit, and an
// unrecoverable failure via Bus.Fatal, which terminates the bus. Goroutines
// started in Open should stop when the ctx given to Open is done.
type Adapter interface {
	Open(ctx context.Context, bus *Bus) error
	// Send writes one frame to the hardware, returning once it has been
	// written (or ctx is done). The Bus never calls Send concurrently.
	Send(ctx context.Context, f Frame) error
	Close() error
}

// Config holds settings applied when an adapter is opened. Not every field
// applies to every adapter.
type Config struct {
	Port          string            // port name, or path to dll/so/dylib
	PortBaudrate  int               // serial port baudrate in bps
	CANRate       float64           // CAN bus rate in kbit/s
	CANFilter     []uint32          // CAN ID filters
	UseExtendedID bool              // use 29-bit IDs when setting up frame filters
	Debug         bool              // enable debug logging
	Extra         map[string]string // adapter specific key/value configuration
}

// Capabilities describes what a registered adapter can do.
type Capabilities struct {
	HSCAN bool
	SWCAN bool
	KLine bool
}

func (c Capabilities) String() string {
	return fmt.Sprintf("HSCAN: %v, SWCAN: %v, KLine: %v", c.HSCAN, c.SWCAN, c.KLine)
}

// AdapterInfo describes a registered adapter: its registry name, what it
// needs and what it can do.
type AdapterInfo struct {
	Name               string
	Description        string
	RequiresSerialPort bool
	SerialPortOptional bool
	Capabilities       Capabilities
	New                func(Config) (Adapter, error)
}

func (a AdapterInfo) String() string {
	return fmt.Sprintf("%s | %s, requires serial port: %v", a.Name, a.Description, a.RequiresSerialPort)
}

var (
	adapterMu  sync.Mutex
	adapterMap = make(map[string]AdapterInfo)
)

// Register adds an adapter to the registry, typically from an init function.
// It fails if the name is already taken.
func Register(info AdapterInfo) error {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	if _, taken := adapterMap[info.Name]; taken {
		return fmt.Errorf("adapter %s already registered", info.Name)
	}
	adapterMap[info.Name] = info
	return nil
}

// NewAdapter constructs the named adapter from the registry with cfg,
// without opening it. Construction only validates configuration — no
// hardware is touched until OpenAdapter — so it is safe to do early, e.g.
// in a settings dialog, and hand the adapter to the code that will open it.
func NewAdapter(adapterName string, cfg Config) (Adapter, error) {
	info, err := lookupAdapter(adapterName)
	if err != nil {
		return nil, err
	}
	a, err := info.New(cfg)
	if err != nil {
		return nil, err
	}
	adapterNames.Store(a, info.Name)
	return a, nil
}

// adapterNames remembers the registry name of adapters constructed by
// NewAdapter, so OpenAdapter (and thus Bus.AdapterName) reports it even for
// adapters opened by instance. Keeping it out of the Adapter interface means
// type assertions on the concrete adapter keep working.
var adapterNames sync.Map // Adapter -> string

// registeredName reports the registry name a NewAdapter-constructed adapter
// was created under.
func registeredName(a Adapter) (string, bool) {
	if v, ok := adapterNames.Load(a); ok {
		return v.(string), true
	}
	return "", false
}

// AdapterName reports the registry name the adapter was constructed under
// (NewAdapter / Open), the adapter's own Name() when it provides one, or the
// Go type name as a last resort. Use it wherever a human-readable adapter
// name is needed before a Bus exists; Bus.AdapterName reports the same name
// after opening.
func AdapterName(a Adapter) string {
	if n, ok := registeredName(a); ok {
		return n
	}
	if n, ok := a.(interface{ Name() string }); ok {
		return n.Name()
	}
	return fmt.Sprintf("%T", a)
}

func lookupAdapter(name string) (AdapterInfo, error) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	info, found := adapterMap[name]
	if !found {
		return AdapterInfo{}, fmt.Errorf("unknown adapter %q", name)
	}
	return info, nil
}

// Adapters returns every registered adapter's info, sorted by name.
func Adapters() []AdapterInfo {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	out := make([]AdapterInfo, 0, len(adapterMap))
	for _, info := range adapterMap {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// AdapterNames returns the names of all registered adapters, sorted
// case-insensitively.
func AdapterNames() []string {
	out := make([]string, 0)
	for _, info := range Adapters() {
		out = append(out, info.Name)
	}
	return out
}
