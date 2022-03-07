## goCAN

A Golang CAN Network stack with modular drivers with simple interface. Uses [go.bug.st/serial](go.bug.st/serial), A cross-platform serial library for go-lang.

Supported devices:

- OBDLink SX
- Lawicel Canusb

## Adapter package

```go
type Adapter interface {
	Init(context.Context) error
	Recv() <-chan CANFrame
	Send() chan<- CANFrame
	Close() error
}
```

## ECU client

```go
type Client interface {
	Info(context.Context, model.ProgressCallback) ([]model.HeaderResult, error)
	PrintECUInfo(context.Context) error
	ResetECU(context.Context, model.ProgressCallback) error
	DumpECU(context.Context, model.ProgressCallback) ([]byte, error)
	FlashECU(context.Context, []byte, model.ProgressCallback) error
	EraseECU(context.Context, model.ProgressCallback) error
}
```
## goCANFlasher

A cross-platform fyne GUI for flashing and dumping Saab Trionic 5/7/8 ECU's

## CANTool

A CMD tool to flash and dump Trionic ECU's

