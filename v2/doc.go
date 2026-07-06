// Package gocan v2 is a CAN bus library with pluggable hardware adapters.
//
// Compared to v1 the API is smaller and more idiomatic:
//
//   - Frame is a plain value (no pointers, no hidden per-send state).
//   - Contexts are the only timeout/cancellation mechanism.
//   - Adapters implement three methods (Open, Send, Close) and push traffic
//     back through the Bus instead of exposing four channels.
//   - Subscriptions are channels or range-over-func iterators whose lifetime
//     is the context they were created with.
//
// Quick start:
//
//	bus, err := gocan.Open(ctx, "loopback", gocan.Config{})
//	if err != nil { ... }
//	defer bus.Close()
//
//	reply, err := bus.Request(ctx, gocan.NewFrame(0x240, data), 0x258)
//
//	for frame := range bus.Frames(ctx, 0x1A0) {
//		fmt.Println(frame)
//	}
//
// This core package is pure stdlib. Hardware adapters live in subpackages
// under adapters/ and register themselves on import.
//
// Migrating from v1? See MIGRATION.md for a complete v1-to-v2 mapping.
package gocan
