module github.com/roffe/gocan

go 1.26.0

require (
	github.com/bendikro/dl v0.0.0-20190410215913-e41fdb9069d4
	github.com/fatih/color v1.19.0
	github.com/google/gousb v1.1.3
	go.bug.st/serial v1.7.1
	golang.org/x/sync v0.20.0
	google.golang.org/grpc v1.76.0
	google.golang.org/protobuf v1.36.10
)

require (
	github.com/Microsoft/go-winio v0.6.2
	go.einride.tech/can v0.16.1
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
)

require (
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	golang.org/x/mod v0.25.0
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/tools v0.34.0 // indirect
)

require golang.org/x/sys v0.46.0

replace go.einride.tech/can => github.com/samuelbrian/can-go v0.0.2
