# C and C++ Debugger

Debug C and C++ programs with `lldb-dap`, which comes with Xcode's command line tools on macOS and with LLVM elsewhere.

Build the program with debug symbols first, then enable the plugin in **Settings → Plugins**:

```sh
cc -g -O0 -o a.out main.c
```

The manifest launches `${workspaceFolder}/a.out`. Change `program` in the plugin's `plugin.json` if the binary has another name or place.

Set a breakpoint in the editor margin or press `F9`, then press `F5`.
