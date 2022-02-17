## cantool t7 info

print ECU info

### Synopsis

Connect to the ECU over CAN and print the info from it

```
cantool t7 info [flags]
```

### Options

```
  -h, --help   help for info
```

### Options inherited from parent commands

```
  -a, --adapter string   what adapter to use (default "canusb")
  -b, --baudrate int     baudrate (default 115200)
  -c, --canrate string   CAN rate in kbit/s, shorts: pbus = 500 (default), ibus = 47.619, t5 = 615.384 (default "500")
  -d, --debug            debug mode
  -p, --port string      com-port, * = print available (default "*")
```

### SEE ALSO

* [cantool t7](cantool_t7.md)	 - Trionic 7 ECU related commands

