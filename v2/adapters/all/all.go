// Package all registers every native v2 adapter, for applications that list
// adapters at runtime (e.g. GUIs). Import it for its side effects:
//
//	import _ "github.com/roffe/gocan/v2/adapters/all"
//
// Applications that use one specific adapter should import that adapter's
// package directly instead, to avoid linking the rest. cgo adapters (combi)
// are not included; import them directly.
package all

import (
	_ "github.com/roffe/gocan/v2/adapters/canlib"
	_ "github.com/roffe/gocan/v2/adapters/canusb"
	_ "github.com/roffe/gocan/v2/adapters/drewtech"
	_ "github.com/roffe/gocan/v2/adapters/elm327"
	_ "github.com/roffe/gocan/v2/adapters/j2534"
	_ "github.com/roffe/gocan/v2/adapters/just4trionic"
	_ "github.com/roffe/gocan/v2/adapters/obdx"
	_ "github.com/roffe/gocan/v2/adapters/pcan"
	_ "github.com/roffe/gocan/v2/adapters/rcan"
	_ "github.com/roffe/gocan/v2/adapters/scantool"
	_ "github.com/roffe/gocan/v2/adapters/slcan"
	_ "github.com/roffe/gocan/v2/adapters/socketcan"
	_ "github.com/roffe/gocan/v2/adapters/txbridge"
	_ "github.com/roffe/gocan/v2/adapters/yaca"
)
