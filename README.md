# CANUSB

Golang implementation of the [Lawicel Ascii API](http://www.can232.com/docs/canusb_manual.pdf) for [CANUSB](https://lawicel-shop.se/elektronik/kommunikation/can/lawicel-canusb-adapter-1m-usb-cable/) adapter running in VCP mode

Thanks to [TrionicTuning](https://www.trionictuning.com/), [Chriva](https://www.trionictuning.com/forum/memberlist.php?mode=viewprofile&u=3231) and [Tomi Liljemark](https://pikkupossu.1g.fi/tomi/tomi.html)

```go
$ go run cmd/main.go  -p com3 -b 921600
2022/01/02 20:27:06 helpers.go:30: Using port: COM3
2022/01/02 20:27:06 helpers.go:32:    USB ID      0403:6001
2022/01/02 20:27:06 helpers.go:33:    USB serial  LW5ZEIRKA
2022/01/02 20:27:06 canusb.go:102:    H/W version V1011
2022/01/02 20:27:06 canusb.go:104:    H/W serial  NY657
2022/01/02 20:27:06 t7.go:76: Trusted access obtained 🥳🎉
2022/01/02 20:27:06 t7.go:85: VIN: YS3DE55H4Y2047605
2022/01/02 20:27:06 t7.go:86: Box HW part number: 5382536  
2022/01/02 20:27:06 t7.go:87: Immo Code: *********
2022/01/02 20:27:06 t7.go:88: Software Saab part number: 5383930
2022/01/02 20:27:06 t7.go:89: ECU Software version: EC0XY3RC.48H
2022/01/02 20:27:06 t7.go:90: Engine type: 9-3 B205E EC2000 EU
2022/01/02 20:27:06 t7.go:91: Tester info: SAAB_PROD_EOL
2022/01/02 20:27:06 t7.go:92: Software date: 021220
2022/01/02 20:27:06 main.go:54: 0x1A0 [13 00 00 32 00 00 64 00]
2022/01/02 20:27:06 main.go:54: 0x1A0 [13 00 00 32 00 00 64 00]
2022/01/02 20:27:06 main.go:54: 0x1A0 [13 00 00 32 00 00 64 00]
2022/01/02 20:27:06 main.go:54: 0x1A0 [13 00 00 32 00 00 64 00]
2022/01/02 20:27:06 main.go:54: 0x1A0 [13 00 00 32 00 00 64 00]
2022/01/02 20:27:06 main.go:60: 0x3A0 [30 05 78 00 00 00 00 00]
2022/01/02 20:27:06 main.go:63: got interrupt, stopping CAN communication
2022/01/02 20:27:06 main.go:44: recv: 2540 sent: 880 errors: 0 dropped : 0
```