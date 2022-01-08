package canusb

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fatih/color"
)

type Frame struct {
	Identifier uint32
	Len        uint8
	Data       []byte
}

func (f *Frame) Byte() []byte {
	//fmt.Println(f.String())
	return []byte(fmt.Sprintf("t%x%d%x\r", f.Identifier, f.Len, f.Data))
}

var (
	yellow = color.New(color.FgYellow).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	green  = color.New(color.FgGreen).SprintfFunc()
	blue   = color.New(color.FgHiBlue).SprintfFunc()
)

func (f *Frame) String() string {
	var out strings.Builder
	out.WriteString(green(fmt.Sprintf("0x%X", f.Identifier)) + " || ")
	for i, b := range f.Data {
		out.WriteString(blue(fmt.Sprintf("%02X", b)))
		if i != len(f.Data)-1 {
			out.WriteString(" ")
		}
	}
	out.WriteString(" || ")

	for i, b := range f.Data {
		out.WriteString(red(fmt.Sprintf("%08b", b)))
		if i != len(f.Data)-1 {
			out.WriteString(" ")
		}
	}
	out.WriteString(" || ")

	out.WriteString(yellow(kex.ReplaceAllString(string(f.Data), ".")))
	return out.String()
}

var kex = regexp.MustCompile("[^A-Za-z0-9]+")
