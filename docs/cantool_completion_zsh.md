## cantool completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions for every new session, execute once:

#### Linux:

	cantool completion zsh > "${fpath[1]}/_cantool"

#### macOS:

	cantool completion zsh > /usr/local/share/zsh/site-functions/_cantool

You will need to start a new shell for this setup to take effect.


```
cantool completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
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

