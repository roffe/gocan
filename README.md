# CANUSB

Golang implementation of the [Lawicel Ascii API](http://www.can232.com/docs/canusb_manual.pdf) for [CANUSB](https://lawicel-shop.se/elektronik/kommunikation/can/lawicel-canusb-adapter-1m-usb-cable/) adapter running in VCP mode

Thanks to [TrionicTuning](https://www.trionictuning.com/), [Chriva](https://www.trionictuning.com/forum/memberlist.php?mode=viewprofile&u=3231) and [Tomi Liljemark](https://pikkupossu.1g.fi/tomi/tomi.html)

## Example tool using the library for Trionic7 ECU dumping and flashing

[Docs](docs/t7.md)

```go
$go run ./cmd/cantool/main.go -p com3 -b 921600 ecu info
port: COM3
   USB ID      0403:6001
   USB serial  LW5ZEIRKA
   H/W version V1011
   H/W serial  NY657
VIN: YS3DE55H4**********
Box HW part number: 5382536  
Immo Code: 8610QH1*********
Software Saab part number: 5383930
ECU Software version: EC0XY3RC.48H
Engine type: 9-3 B205E EC2000 EU
Tester info: SAAB_PROD_EOL
Software date: 021220
```