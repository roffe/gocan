## cantool completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(cantool completion bash)

To load completions for every new session, execute once:

#### Linux:

	cantool completion bash > /etc/bash_completion.d/cantool

#### macOS:

	cantool completion bash > /usr/local/etc/bash_completion.d/cantool

You will need to start a new shell for this setup to take effect.


```
cantool completion bash
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
  -c, --canrate string   CAN rate in kbit/s, shorts: pbus = 500 (default), ibus = 47.619, t5 = 615.384 (default "500")
  -d, --debug            debug mode
  -p, --port string      com-port, * = print available (default "*")
```

### SEE ALSO

* [cantool completion](cantool_completion.md)	 - Generate the autocompletion script for the specified shell

