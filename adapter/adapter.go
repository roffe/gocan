package adapter

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/roffe/gocan"
)

var (
	ErrDroppedFrame = fmt.Errorf("incoming buffer full")

	adapterMap = make(map[string]*AdapterInfo)
)

// We have 3 bits allowing 8 different system messages hidden in a 29bit can id stored in a uint32
const (
	SystemMsg uint32 = 0x80000000 + iota
	SystemMsgError
	SystemMsgDebug
	SystemMsgWBLReading
	SystemMsgDataResponse
	SystemMsgDataRequest
	SystemMsgWriteResponse
)

type token struct{}

type AdapterFunc func(*gocan.AdapterConfig) (gocan.Adapter, error)

type AdapterInfo struct {
	Name               string
	Description        string
	Capabilities       AdapterCapabilities
	RequiresSerialPort bool
	New                AdapterFunc
}

type AdapterCapabilities struct {
	HSCAN bool
	SWCAN bool
	KLine bool
}

func New(adapterName string, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	if cfg.OnMessage == nil {
		cfg.OnMessage = func(s string) {
			log.Println(s)
		}
	}
	if cfg.OnError == nil {
		cfg.OnError = func(err error) {
			log.Println(err)
		}
	}
	if adapter, found := adapterMap[adapterName]; found {
		return adapter.New(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

func Register(adapter *AdapterInfo) error {
	if _, found := adapterMap[adapter.Name]; !found {
		adapterMap[adapter.Name] = adapter
		return nil
	}
	return fmt.Errorf("adapter %s already registered", adapter.Name)
}

func List() []string {
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

func GetAdapterMap() map[string]*AdapterInfo {
	return adapterMap
}
