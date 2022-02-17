## cantool completion powershell

Generate the autocompletion script for powershell

### Synopsis

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	cantool completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.


```
cantool completion powershell [flags]
```

### Options

```
  -h, --help              help for powershell
      --no-descriptions   disable completion descriptions
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

* [cantool completion](cantool_completion.md)	 - Generate the autocompletion script for the specified shell

