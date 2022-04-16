# goCAN

A Go linux/windows/osx CAN stack running in userland with low level adapter drivers written from the ground up

Supported adapters:
* OBDLinx SX
* [CANUSB](https://lawicel-shop.se/elektronik/kommunikation/can/lawicel-canusb-adapter-1m-usb-cable/) adapter running in VCP mode using [Lawicel ascii api](http://www.can232.com/docs/canusb_manual.pdf)


## Included tools in repo

### goCANFlasher

Trionic 5/7/8 ecu flasher GUI made with Fyne, [**Nightmare!**](https://doom.fandom.com/wiki/Skill_level#Doom.2C_Doom_II_and_Final_Doom_skill_levels)

Most Trionic code is based on [TrionicCANFlasher](https://txsuite.org/)

### cantool

CLI tool for CAN monitoring and ECU flashing

#### Example

```go
go run .\cmd\cantool\main.go -p com3 -b 2000000 -a canusb -c t5 -t t5 info
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

## Credits

Thanks to [TrionicTuning](https://www.trionictuning.com/), [Chriva](https://www.trionictuning.com/forum/memberlist.php?mode=viewprofile&u=3231) and [Tomi Liljemark](https://pikkupossu.1g.fi/tomi/tomi.html)