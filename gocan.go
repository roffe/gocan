//go:generate protoc.exe --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative .\proto\server.proto

package gocan

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// ErrClosed is the context cause set when a client is shut down via Close. It is
// reported as a clean shutdown (Client.Err returns nil) rather than a fatal error.
var ErrClosed = errors.New("gocan: client closed")

// We have 3 bits allowing 8 different system messages hidden in a 29bit can id stored in a uint32
const (
	SystemMsg uint32 = 0x80000000 + iota
	SystemMsgError
	SystemMsgUnrecoverableError
	SystemMsgWBLReading
	SystemMsgDataResponse
	SystemMsgDataRequest
	SystemMsgWriteResponse
	SystemMsgUnknown
)

type Opt func(*Client)

// eventSink wraps a registered event callback. A pointer identity lets OnEvent
// hand back a working unregister function (func values are not comparable).
type eventSink struct {
	fn func(Event)
}

type Client struct {
	fh      *handler
	adapter Adapter

	// ctx is cancelled when the client terminates. The cancel cause carries the
	// fatal adapter error, or ErrClosed on a clean shutdown.
	ctx    context.Context
	cancel context.CancelCauseFunc

	sinkMu sync.Mutex
	sinks  []*eventSink

	closeOnce sync.Once
}

// WithEventFunc registers fn as a synchronous event handler. It is called in
// order, from a single goroutine, for every adapter event (including the final
// fatal event). Keep it quick; a slow handler delays subsequent events.
func WithEventFunc(fn func(Event)) Opt {
	return func(c *Client) { c.addSink(fn) }
}

// WithEventChan forwards every adapter event to ch. Sends are non-blocking: if
// ch is full the event is dropped, so size the buffer for your consumer. The
// channel is never closed by the client; range with a select on Client.Done.
func WithEventChan(ch chan<- Event) Opt {
	return func(c *Client) {
		c.addSink(func(e Event) {
			select {
			case ch <- e:
			default:
			}
		})
	}
}

// WithLogger forwards every adapter event to l at the matching slog level.
// Error and fatal events attach the underlying error under the "err" key.
func WithLogger(l *slog.Logger) Opt {
	return func(c *Client) {
		c.addSink(func(e Event) {
			var attrs []slog.Attr
			if e.Err != nil {
				attrs = append(attrs, slog.Any("err", e.Err))
			}
			l.LogAttrs(context.Background(), e.Type.Level(), e.Details, attrs...)
		})
	}
}

// Deprecated: use WithEventFunc. Retained for backwards compatibility; note the
// callback is now invoked synchronously and in order rather than per-goroutine.
func WithEventHandler(fn func(Event)) Opt {
	return WithEventFunc(fn)
}

// Create a new CAN client with given adapter name and config
func New(ctx context.Context, adapterName string, cfg *AdapterConfig) (*Client, error) {
	adapter, err := NewAdapter(adapterName, cfg)
	if err != nil {
		return nil, err
	}
	return NewWithOpts(ctx, adapter)
}

// Create a new CAN client with given adapter and options
func NewWithOpts(ctx context.Context, adapter Adapter, opts ...Opt) (*Client, error) {
	cctx, cancel := context.WithCancelCause(ctx)
	c := &Client{
		fh:      newHandler(adapter),
		adapter: adapter,
		ctx:     cctx,
		cancel:  cancel,
	}
	for _, opt := range opts {
		opt(c)
	}
	go c.fh.run(cctx)
	go c.pump()
	if err := adapter.Open(ctx); err != nil {
		c.cancel(err)
		return nil, err
	}
	return c, nil
}

// addSink registers an event callback and returns its handle.
func (c *Client) addSink(fn func(Event)) *eventSink {
	if fn == nil {
		return nil
	}
	s := &eventSink{fn: fn}
	c.sinkMu.Lock()
	// Append onto a fresh slice so pump can snapshot the header lock-free.
	c.sinks = append(c.sinks[:len(c.sinks):len(c.sinks)], s)
	c.sinkMu.Unlock()
	return s
}

// OnEvent registers fn to receive subsequent events and returns a function that
// unregisters it. Unlike the construction-time options it can be called at any
// time and supports multiple independent listeners.
func (c *Client) OnEvent(fn func(Event)) (cancel func()) {
	s := c.addSink(fn)
	if s == nil {
		return func() {}
	}
	return func() {
		c.sinkMu.Lock()
		defer c.sinkMu.Unlock()
		for i, cur := range c.sinks {
			if cur == s {
				next := make([]*eventSink, 0, len(c.sinks)-1)
				next = append(next, c.sinks[:i]...)
				next = append(next, c.sinks[i+1:]...)
				c.sinks = next
				return
			}
		}
	}
}

// emit delivers an event to every registered sink, in registration order.
func (c *Client) emit(e Event) {
	c.sinkMu.Lock()
	sinks := c.sinks
	c.sinkMu.Unlock()
	for _, s := range sinks {
		s.fn(e)
	}
}

// pump drains adapter events and the terminal fatal error on a single
// goroutine, guaranteeing in-order delivery. A fatal error is surfaced as a
// final EventTypeFatal event and cancels the client context with that error as
// the cause.
func (c *Client) pump() {
	events := c.adapter.Event()
	errs := c.adapter.Err()
	for {
		select {
		case <-c.ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			c.emit(e)
		case err, ok := <-errs:
			if !ok {
				return
			}
			if err != nil {
				c.emit(Event{Type: EventTypeFatal, Details: err.Error(), Err: err})
				c.cancel(err)
			}
			return
		}
	}
}

// Return the underlying adapter
func (c *Client) Adapter() Adapter {
	return c.adapter
}

// Return the name of the underlying adapter
func (c *Client) AdapterName() string {
	return c.adapter.Name()
}

// Context returns the client context. It is cancelled when the client is closed
// or the adapter fails fatally; context.Cause reports why (ErrClosed on a clean
// shutdown, otherwise the fatal adapter error).
func (c *Client) Context() context.Context {
	return c.ctx
}

// Done returns a channel that is closed when the client terminates, for use in
// a select alongside your own work.
func (c *Client) Done() <-chan struct{} {
	return c.ctx.Done()
}

// Err returns the fatal adapter error that terminated the client, or nil if it
// is still running or was shut down cleanly (via Close or context cancellation).
func (c *Client) Err() error {
	cause := context.Cause(c.ctx)
	switch {
	case cause == nil,
		errors.Is(cause, ErrClosed),
		errors.Is(cause, context.Canceled),
		errors.Is(cause, context.DeadlineExceeded):
		return nil
	default:
		return cause
	}
}

// Wait blocks until ctx is done or the client terminates, returning the fatal
// adapter error if one occurred (nil on a clean shutdown or ctx cancellation).
func (c *Client) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		if err := c.Err(); err != nil {
			return err
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return ctx.Err()
	case <-c.ctx.Done():
		return c.Err()
	}
}

// Close the client and underlying adapter
func (c *Client) Close() (err error) {
	c.closeOnce.Do(func() {
		err = c.adapter.Close()
		c.cancel(ErrClosed)
		c.fh.close()
	})
	return
}

// Send a CAN Frame
func (c *Client) SendFrame(msg *CANFrame) error {
	select {
	case c.adapter.Send() <- msg:
		return nil
	case <-c.ctx.Done():
		if err := c.Err(); err != nil {
			return err
		}
		return ErrClosed
	case <-time.After(5 * time.Second):
		return &TimeoutError{
			Timeout: 5,
			Frames:  []uint32{msg.Identifier},
			Type:    "send",
		}
	}
}

// SendSync sends a frame and blocks until the adapter has written it to the
// hardware (or ctx / timeout fires). This gives natural inter-frame pacing that
// plain async Send does not, since Send only queues into the adapter buffer.
// On adapters that don't confirm write-completion it degrades to a plain Send.
func (c *Client) SendSync(ctx context.Context, frame *CANFrame, timeout time.Duration) error {
	if sc, ok := c.adapter.(interface{ SupportsSync() bool }); !ok || !sc.SupportsSync() {
		return c.SendFrame(frame)
	}
	frame.sent = make(chan struct{}, 1)
	if err := c.SendFrame(frame); err != nil {
		return err
	}
	select {
	case <-frame.sent:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return ErrClosed
	case <-time.After(timeout):
		return nil // safety net; a sync-capable adapter should always signal
	}
}

// Shortcommand to send a standard 11bit frame
func (c *Client) Send(identifier uint32, data []byte, f CANFrameType) error {
	return c.SendFrame(NewFrame(identifier, data, f))
}

// Shortcommand to send a extended 29bit frame
func (c *Client) SendExtended(identifier uint32, data []byte, f CANFrameType) error {
	return c.SendFrame(NewExtendedFrame(identifier, data, f))
}

// Send and wait up to <timeout> for a answer on given identifiers
func (c *Client) SendAndWait(ctx context.Context, frame *CANFrame, timeout time.Duration, identifiers ...uint32) (*CANFrame, error) {
	frame.Timeout = uint32(timeout.Milliseconds())
	sub := c.newSub(1, identifiers...)
	defer func() {
		c.fh.unregisterSub(sub)
	}()
	if err := c.SendFrame(frame); err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sub.wait(waitCtx)
}

// Receive a single CAN frame with specific identifier and timeout
func (c *Client) Recv(ctx context.Context, timeout time.Duration, identifiers ...uint32) (*CANFrame, error) {
	sub := c.newSub(1, identifiers...)
	defer func() {
		c.fh.unregisterSub(sub)
	}()
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sub.wait(waitCtx)
}

// Subscribe to CAN identifiers with a callback function
func (c *Client) SubscribeFunc(ctx context.Context, fn func(*CANFrame), identifiers ...uint32) *Subscriber {
	sub := c.Subscribe(ctx, identifiers...)
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-sub.responseChan:
				if !ok {
					return
				}
				fn(frame)
			}
		}
	}()
	return sub
}

// Subscribe to CAN identifiers with provided channel
func (c *Client) SubscribeChan(ctx context.Context, channel chan *CANFrame, identifiers ...uint32) *Subscriber {
	sub := &Subscriber{
		cl:           c,
		identifiers:  toSet(identifiers),
		filterCount:  len(identifiers),
		responseChan: channel,
	}
	c.fh.registerSub(sub)
	return sub
}

// Subscribe to CAN identifiers and return a message channel
func (c *Client) Subscribe(ctx context.Context, identifiers ...uint32) *Subscriber {
	return c.newSub(40, identifiers...)
}

func (c *Client) newSub(bufferSize int, identifiers ...uint32) *Subscriber {
	sub := &Subscriber{
		createdAt:    callerInfo(3),
		cl:           c,
		identifiers:  toSet(identifiers),
		filterCount:  len(identifiers),
		responseChan: make(chan *CANFrame, bufferSize),
	}
	c.fh.registerSub(sub)
	return sub
}

func callerInfo(depth int) string {
	_, file, line, ok := runtime.Caller(depth)
	if !ok {
		return "unknown"
	}
	//fn := runtime.FuncForPC(pc)
	//if fn != nil {
	//	funcName = fn.Name()
	//}
	return filepath.Base(file) + ":" + strconv.Itoa(line)
}

func toSet(identifiers []uint32) map[uint32]struct{} {
	idMap := make(map[uint32]struct{}, len(identifiers))
	for _, id := range identifiers {
		idMap[id] = struct{}{}
	}
	return idMap
}
