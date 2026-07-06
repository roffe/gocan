package gocan

import (
	"context"
	"time"
)

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

type responseTimeoutKey struct{}

// WithResponseTimeout hints buffered adapters (the ELM/STN family) how long
// to wait on the wire for the hinted reply frames of a single exchange — the
// v2 form of v1's per-command timeout variable. Without it such adapters use
// their own default (250 ms). Request stamps it automatically from its
// context deadline when that deadline is near; the ctx deadline itself is
// never used as a wire timeout (an operation-lifetime deadline says nothing
// about one frame's reply).
func WithResponseTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, responseTimeoutKey{}, d)
}

// ResponseTimeout returns the reply-wait hint carried by ctx, or 0 when none
// is set. For adapter implementations.
func ResponseTimeout(ctx context.Context) time.Duration {
	d, _ := ctx.Value(responseTimeoutKey{}).(time.Duration)
	return d
}
