## t7 completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	t7 completion fish | source

To load completions for every new session, execute once:

	t7 completion fish > ~/.config/fish/completions/t7.fish

You will need to start a new shell for this setup to take effect.


```
t7 completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
  -b, --baudrate int   baudrate (default 115200)
  -d, --debug          debug mode
  -p, --port string    com-port, * = print available (default "*")
```

### SEE ALSO

* [t7 completion](t7_completion.md)	 - Generate the autocompletion script for the specified shell

