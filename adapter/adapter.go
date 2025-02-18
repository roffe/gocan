package adapter

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/roffe/gocan"
)

var (
	ErrDroppedFrame = fmt.Errorf("incoming buffer full")

	adapterMap = make(map[string]*AdapterInfo)
)

type AdapterInfo struct {
	Name               string
	Description        string
	Capabilities       AdapterCapabilities
	RequiresSerialPort bool
	New                func(*gocan.AdapterConfig) (gocan.Adapter, error)
}

type AdapterCapabilities struct {
	HSCAN bool
	SWCAN bool
	KLine bool
}

func New(adapterName string, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	if cfg.OnMessage == nil {
		cfg.OnMessage = func(msg string) {
			_, file, no, ok := runtime.Caller(1)
			if ok {
				fmt.Printf("%s#%d %v\n", filepath.Base(file), no, msg)
			} else {
				log.Println(msg)
			}
		}
	}
	/*
		if cfg.OnError == nil {
			cfg.OnError = func(err error) {
				_, file, no, ok := runtime.Caller(1)
				if ok {
					fmt.Printf("%s#%d %v\n", filepath.Base(file), no, err)
				} else {
					log.Println(err)
				}
			}
		}
	*/
	if adapter, found := adapterMap[adapterName]; found {
		return adapter.New(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

func Register(adapter *AdapterInfo) error {
	//log.Println("Registering adapter", adapter.Name)
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
