// Package rcan drives the rCAN USB device (roffe.nu, VID:PID ffff:1337).
// Opt-in with the "rcan" build tag since it needs libusb via gousb (cgo);
// without the tag the package is empty.
package rcan
