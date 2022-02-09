package gocan

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
)

type Frame struct {
	Time       time.Time
	Identifier uint32
	Len        uint8
	Data       []byte
}

func (f *Frame) Byte() []byte {
	//fmt.Println(f.String())
	return []byte(fmt.Sprintf("t%x%d%x\r", f.Identifier, f.Len, f.Data))
}

var (
	yellow = color.New(color.FgHiBlue).SprintfFunc()
	red    = color.New(color.FgRed).SprintfFunc()
	green  = color.New(color.FgGreen).SprintfFunc()
)

func (f *Frame) String() string {
	var out strings.Builder
	out.WriteString(green("0x%03X", f.Identifier) + " || ")

	var hexView strings.Builder

	for i, b := range f.Data {
		hexView.WriteString(fmt.Sprintf("%02X", b))
		if i != len(f.Data)-1 {
			hexView.WriteString(" ")
		}
	}

	out.WriteString(fmt.Sprintf("%-23s", hexView.String()))

	out.WriteString(" || ")

	var binView strings.Builder
	for i, b := range f.Data {
		binView.WriteString(fmt.Sprintf("%08b", b))
		if i != len(f.Data)-1 {
			binView.WriteString(" ")
		}
	}

	out.WriteString(red(fmt.Sprintf("%-72s", binView.String())))

	out.WriteString(" || ")
	out.WriteString(yellow("%8s", kex.ReplaceAllString(string(f.Data), ".")))
	return out.String()
}

var kex = regexp.MustCompile("[^A-Za-z0-9.,!?]+")
