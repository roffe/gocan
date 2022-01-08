## t7 completion powershell

Generate the autocompletion script for powershell

### Synopsis

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	t7 completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.


```
t7 completion powershell [flags]
```

### Options

```
  -h, --help              help for powershell
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

