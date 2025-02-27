# goCAN

A Go Linux/Windows/OSX CAN library

## Build tags

* combi - requires libusb-1.0.dll
* canlib - requires client32.dll
* canusb - requires canusbdrv(64).dll
* j2538 - requires vendor DLL to be installed

## Usage

Get goCAN

	go get github.com/roffe/gocan@latest

Import the package in your imports

	import (
		...
		"github.com/roffe/gocan"
	)

## Showcase

* [Saab CAN flasher](https://github.com/roffe/gocanflasher)
* [txlogger](https://github.com/roffe/txlogger)

## Supported Adapters

### USB Serial

* OBDLinx SX/EX/MX/MX+: https://www.obdlink.com/
* STN1130
* STN1170
* STN2120
* [CANUSB](https://www.canusb.com/products/canusb/) adapter running in VCP mode using [Lawicel ascii api](https://www.canusb.com/files/canusb_manual.pdf)
* CANable Nano and Pro running [slcan](https://github.com/normaldotcom/canable-fw)
* [YACA](https://github.com/roffe/yaca)

### libusb

* CombiAdapter

### Canusb DLL

Supported via [goCANUSB](https://github.com/roffe/gocanusb)

Lawicel canusbdrv.dll for both 32 and 64bit is supported

### Kvaser Canlib

Supported via [goCANlib](https://github.com/roffe/gocanlib), Tested with the following adapters

* Kvaser Leaf Light V2 https://kvaser.com/

### J2534

Support for both 32 & 64bit DLL's. Your GOARCH will controll which it will look for.

Do note that not all vendors provide 64bit DLL's so you migh need to build your software with GOARCH=386 to be able to use the j2534 DLL

Most adapters that comes with a J2534 DLL will work. The list given is just ones verified to work.

#### Windows

* Drewtech Mongoose GM PRO II: https://www.drewtech.com/
* Scanmatik 2 PRO: https://scanmatik.pro/
* Tech2 passthru ( limited support )
* GM MDI
* OBDX Pro GT/VT: https://www.obdxpro.com/
* Kvaser Leaf Light V2: https://kvaser.com/
* PCAN32

#### Linux
* Tactrix Openport 2.0: https://github.com/dschultzca/j2534
* Machina: https://github.com/rnd-ash/Macchina-J2534
* WQCAN: https://github.com/witoldo7/STM32CAN

### Other

* SocketCAN
* OBDX Pro GT BLE & Wifi
* Combiadapter via libusb
