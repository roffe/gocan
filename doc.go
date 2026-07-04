// Package gocan is a CAN bus client library with interchangeable hardware
// adapter backends: ELM327/STN serial dongles, SocketCAN, J2534 passthru
// devices, Kvaser, PCAN, CombiAdapter and more.
//
// A Client is created with New (adapter looked up by registry name, see
// ListAdapterNames) or NewWithOpts (adapter constructed by the caller):
//
//	c, err := gocan.New(ctx, "ELM327", &gocan.AdapterConfig{Port: "COM3", CANRate: 500})
//
// Frames are sent with Send, SendFrame, SendAndWait (request/response) and
// SendSync (blocks until the frame reaches the hardware). Frames are received
// one-shot with Recv or streamed with Subscribe, SubscribeFunc and
// SubscribeChan, filtered on CAN identifiers.
//
// # Frame ownership
//
// Outgoing frames are single-use: send methods attach per-send state to the
// frame, so build a fresh frame for every send. Frames received from the bus
// are shared by every matching subscriber and must be treated as read-only,
// including the Data slice.
//
// # Lifecycle
//
// The Client owns its adapter. Close shuts both down. If the adapter fails
// fatally the client's context is cancelled: Done, Err and Wait report the
// failure, and every registered event listener receives a final
// EventTypeFatal event. Recoverable adapter noise is delivered as Events via
// the WithEventFunc, WithEventChan and WithLogger options or OnEvent.
//
// Some backends require build tags (combi, j2534, canlib, canusb, pcan,
// ftdi); see the README for the full matrix and supported hardware.
package gocan
