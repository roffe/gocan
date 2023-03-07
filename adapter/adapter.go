package adapter

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/roffe/gocan"
)

var (
	debug      bool
	adapterMap = make(map[string]AdapterFunc)
)

func init() {
	if strings.ToLower(os.Getenv("DEBUG")) == "true" {
		debug = true
	}
}

type AdapterFunc func(*gocan.AdapterConfig) (gocan.Adapter, error)
type token struct{}

func New(adapterName string, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	if cfg.OutputFunc == nil {
		cfg.OutputFunc = func(s string) {
			log.Println(s)
		}
	}
	if cfg.ErrorFunc == nil {
		cfg.ErrorFunc = func(err error) {
			log.Println(err)
		}
	}
	if adapter, found := adapterMap[adapterName]; found {
		return adapter(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

func Register(name string, adapter AdapterFunc) error {
	if _, found := adapterMap[name]; !found {
		adapterMap[name] = adapter
		return nil
	}
	return fmt.Errorf("adapter %s already registered", name)
}

func List() []string {
	var out []string
	for name := range adapterMap {
		out = append(out, name)
	}
	return out
}
