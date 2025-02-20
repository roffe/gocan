package canusb

import "syscall"

var (
	canusbdrv = syscall.NewLazyDLL("canusbdrv64.dll")
)
