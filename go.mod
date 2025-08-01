module github.com/roffe/gocan

// replace github.com/roffe/gocanusb => ..\gocanusb

// replace github.com/roffe/gocanlib => ..\gocanlib

// replace github.com/roffe/goftdi => ..\goftdi

go 1.24.0

require (
	github.com/albenik/bcd v0.0.0-20170831201648-635201416bc7
	github.com/bendikro/dl v0.0.0-20190410215913-e41fdb9069d4
	github.com/fatih/color v1.18.0
	github.com/google/gousb v1.1.3
	go.bug.st/serial v1.6.4
	go.einride.tech/can v0.12.1
	golang.org/x/sync v0.11.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
)

require github.com/roffe/goftdi v0.0.0-20250330120219-31feb988cb73

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/roffe/gocanusb v1.1.2
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250207221924-e9438ea467c6 // indirect
)

require (
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	golang.org/x/mod v0.23.0
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	golang.org/x/sys v0.30.0
)
