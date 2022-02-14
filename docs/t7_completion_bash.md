## t7 completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(t7 completion bash)

To load completions for every new session, execute once:

#### Linux:

	t7 completion bash > /etc/bash_completion.d/t7

#### macOS:

	t7 completion bash > /usr/local/etc/bash_completion.d/t7

You will need to start a new shell for this setup to take effect.


```
t7 completion bash
```

### Options

```
  -h, --help              help for bash
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

