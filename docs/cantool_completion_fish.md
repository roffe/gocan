## cantool completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	cantool completion fish | source

To load completions for every new session, execute once:

	cantool completion fish > ~/.config/fish/completions/cantool.fish

You will need to start a new shell for this setup to take effect.


```
cantool completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
  -a, --adapter string   what adapter to use (default "canusb")
  -b, --baudrate int     baudrate (default 115200)
  -c, --canrate string   CAN rate in kbit/s, shorts: pbus = 500, ibus = 47.619, t5 = 615.384 (default "500")
  -d, --debug            debug mode
  -p, --port string      com-port, * = print available (default "*")
```

### SEE ALSO

* [cantool completion](cantool_completion.md)	 - Generate the autocompletion script for the specified shell

