# CANTOOL

[Docs](docs/cantool.md)

Golang linux/windows/osx Trionic 5/7 ecu flasher

Supported adapters:
* OBDLinx SX ( not fully tested )
* [CANUSB](https://lawicel-shop.se/elektronik/kommunikation/can/lawicel-canusb-adapter-1m-usb-cable/) adapter running in VCP mode using [Lawicel ascii api](http://www.can232.com/docs/canusb_manual.pdf)

Most code is based on [TrionicCANFlasher](https://txsuite.org/)

Thanks to [TrionicTuning](https://www.trionictuning.com/), [Chriva](https://www.trionictuning.com/forum/memberlist.php?mode=viewprofile&u=3231) and [Tomi Liljemark](https://pikkupossu.1g.fi/tomi/tomi.html)

## Example

```go
go run .\cmd\cantool\main.go -p com3 -b 2000000 -a canusb -c t5 t5 info
uploading bootloader 100% [====================] (1.793 kB/s) took: 1.029s
------------------------------
This is a Trionic 5.5 ECU with 256 kB of FLASH
----- ECU info ---------------
Part Number:  4780656
Software ID:  4782934
SW Version:   A554X24L.18C
Engine Type:  B204LM 900    ROFFE ST2
IMMO Code:    ******
Other Info:   LX1
ROM Start:    0x040000
Code End:     0x076D39
ROM End:      0x07FFFF
------------------------------
Resetting ECU
```