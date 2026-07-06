package gocan_test

import (
	"context"
	"fmt"

	gocan "github.com/roffe/gocan/v2"
)

func Example() {
	ctx := context.Background()
	bus, err := gocan.Open(ctx, "loopback", gocan.Config{})
	if err != nil {
		panic(err)
	}
	defer bus.Close()

	reply, err := bus.Request(ctx, gocan.NewFrame(0x123, []byte("hello")), 0x123)
	if err != nil {
		panic(err)
	}
	fmt.Printf("0x%03X %s\n", reply.ID, reply.Bytes())
	// Output: 0x123 hello
}
