package gocan

import (
	"context"
)

type Template struct {
	*BaseAdapter
}

func NewTemplate(name string, cfg *AdapterConfig) (Adapter, error) {
	return &Template{
		BaseAdapter: NewBaseAdapter(name, cfg),
	}, nil
}

func (a *Template) Open(ctx context.Context) error {
	return nil
}

func (a *Template) SetFilter(filters []uint32) error {
	return nil
}

func (a *Template) Close() error {
	a.BaseAdapter.Close()
	return nil
}
