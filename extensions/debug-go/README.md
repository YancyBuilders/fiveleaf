# Go Debugger

Debug Go programs with [Delve](https://github.com/go-delve/delve)'s DAP server.

Install `dlv` on the machine that holds the folder, then enable the plugin in **Settings → Plugins**:

```sh
go install github.com/go-delve/delve/cmd/dlv@latest
```

Set a breakpoint in the editor margin or press `F9`, then press `F5`. The Debug view shows the call stack, variables and the program's output.
