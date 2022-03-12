package model

import "fmt"

type ProgressCallback func(interface{})

type Header struct {
	Desc string
	ID   uint8
	Type string
}

type HeaderResult struct {
	Header
	Value string
}

func (t *HeaderResult) String() string {
	return t.Desc + ": " + t.Value
}

type DTC struct {
	Code   string
	Status byte
}

func (d *DTC) String() string {
	return fmt.Sprintf("%s, Status: %X", d.Code, d.Status)
}
