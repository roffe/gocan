package adapter

import (
	"context"

	"github.com/roffe/gocan"
)

type Template struct {
	*BaseAdapter
}

func NewTemplate(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Template{
		BaseAdapter: NewBaseAdapter(cfg),
	}, nil
}

func (a *Template) SetFilter(filters []uint32) error {
	return nil
}

func (a *Template) Name() string {
	return "Template"
}

func (a *Template) Init(ctx context.Context) error {
	return nil
}

func (a *Template) Close() error {
	a.BaseAdapter.Close()
	return nil
}
