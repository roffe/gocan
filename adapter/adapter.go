package adapter

import (
	"fmt"

	"github.com/roffe/gocan"
)

var adapterMap = make(map[string]NewAdapterFunc)

type AdapterItem struct {
	Name string
	New  NewAdapterFunc
}

type NewAdapterFunc func(*gocan.AdapterConfig) (gocan.Adapter, error)

func New(adapterName string, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	if adapter, found := adapterMap[adapterName]; found {
		return adapter(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

func List() []string {
	var out []string
	for name := range adapterMap {
		out = append(out, name)
	}
	return out
}

func RegisterAdapter(name string, initFunc NewAdapterFunc) {
	if _, ok := adapterMap[name]; ok {
		panic("adapter already registered")
	}
	adapterMap[name] = initFunc
}
