package gocan

import "context"

type expectedResponsesKey struct{}

// WithExpectedResponses hints buffered adapters (the ELM/STN family) that a
// Send using this context should gather n reply frames before returning to
// command mode. Adapters without reply buffering ignore it.
//
// Request stamps a hint of 1 automatically; set a higher count for commands
// that answer with multiple frames:
//
//	ctx = gocan.WithExpectedResponses(ctx, 4)
//	first, err := bus.Request(ctx, frame, 0x258)
func WithExpectedResponses(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, expectedResponsesKey{}, n)
}

// ExpectedResponses returns the reply-count hint carried by ctx, or 0 when
// none is set. For adapter implementations.
func ExpectedResponses(ctx context.Context) int {
	n, _ := ctx.Value(expectedResponsesKey{}).(int)
	return n
}
