package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/roffe/gocan"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	for i, adapter := range gocan.ListAdapters() {
		fmt.Printf("#%d %s\n", i, adapter.String())
		fmt.Println(adapter.Capabilities.String())
		fmt.Println(strings.Repeat("-", 30))
	}
}
