# JavaScript and TypeScript Debugger

Set a breakpoint in the editor gutter or with `F9`. Press `F5` to launch the active JavaScript or TypeScript file with Node.js, then `F10` to step over the current line. The plugin uses Microsoft's standalone js-debug server. FiveLeaf opens the child DAP session requested for the Node.js target. [Upstream DAP server](https://github.com/microsoft/vscode-js-debug/blob/v1.140.1/src/dapDebugServer.ts)

## Local setup

Install the plugin with `ff plugin install debug-js`. For version `0.1.1`, run the following commands in the installed `debug-js` plugin directory. They create `js-debug/src/dapDebugServer.js` in that directory. [Tested upstream release](https://github.com/microsoft/vscode-js-debug/releases/tag/v1.140.1)

```sh
curl -fLO https://github.com/microsoft/vscode-js-debug/releases/download/v1.140.1/js-debug-dap-v1.140.1.tar.gz
tar -xzf js-debug-dap-v1.140.1.tar.gz
```

Enable **JavaScript and TypeScript Debugger** in **Settings → Plugins**. The plugin looks for `node` on the local `PATH`. The `nodePath` setting selects the executable that starts the adapter. To debug `.ts` files directly, use a Node.js version that supports type stripping and source syntax it can execute. The plugin was tested with Node.js 26.8.1.
