package gocan

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
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
	//SetFilter([]uint32) error
}

type AdapterInfo struct {
	Name               string
	Description        string
	Capabilities       AdapterCapabilities
	RequiresSerialPort bool
	New                func(*AdapterConfig) (Adapter, error)
}

type AdapterCapabilities struct {
	HSCAN bool
	SWCAN bool
	KLine bool
}

type AdapterConfig struct {
	Debug                  bool
	Port                   string
	PortBaudrate           int
	CANRate                float64
	CANFilter              []uint32
	UseExtendedID          bool
	PrintVersion           bool
	OnMessage              func(string)
	MinimumFirmwareVersion string
}

var adapterMap = make(map[string]*AdapterInfo)

func NewAdapter(adapterName string, cfg *AdapterConfig) (Adapter, error) {
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

func RegisterAdapter(adapter *AdapterInfo) error {
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
