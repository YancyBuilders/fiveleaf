# Rust Debugger

Debug Rust programs with `lldb-dap`, which comes with Xcode's command line tools on macOS and with LLVM elsewhere.

Build the program first, then enable the plugin in **Settings → Plugins**:

```sh
cargo build
```

The manifest launches `${workspaceFolder}/target/debug/app`; change `program` for another binary or package name.

Set a breakpoint in the editor margin or press `F9`, then press `F5`.
