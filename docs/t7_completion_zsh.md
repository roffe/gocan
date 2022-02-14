## t7 completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions for every new session, execute once:

#### Linux:

	t7 completion zsh > "${fpath[1]}/_t7"

#### macOS:

	t7 completion zsh > /usr/local/share/zsh/site-functions/_t7

You will need to start a new shell for this setup to take effect.


```
t7 completion zsh [flags]
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
  -d, --debug            debug mode
  -p, --port string      com-port, * = print available (default "*")
```

### SEE ALSO

* [t7 completion](t7_completion.md)	 - Generate the autocompletion script for the specified shell

