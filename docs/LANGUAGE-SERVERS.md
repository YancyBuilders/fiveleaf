# Language servers

Install each language-server plugin by name in **Settings → Plugins** or with `ff plugin install`, for example
`ff plugin install lsp-go`. Install its server on the workspace machine, then enable the plugin. Opening unrelated files never
installs packages or starts servers for other languages. A server starts when you open a matching file in a workspace.

The presets provide the server's diagnostics, completion, hover, go-to-definition and formatting where that server
supports them. They do not add LSP features the editor has not implemented. The status-bar language-server icon shows
startup errors and offers **Restart** and **Stop** for one server or all servers in the current workspace.
The HTML, CSS and JSON presets explicitly enable their servers' built-in formatters.

Code suggestions appear at the caret after a typing pause, including after a trigger such as `.` when the server
advertises it. **Selection → Complete Word** opens the searchable list on demand. Nearby words in the document
provide suggestions when the server has no matches. **Selection → Format Document** uses a formatter plugin first,
then the language server, with built-in formatting available for Go and strict JSON.

## Choose a formatter

Install the tool on the machine that holds the workspace, then install and enable its formatter in **Settings → Plugins**.
Press `Shift+Alt+F` to format the current buffer, or turn on **Format on Save** there. You can enable a formatter
independently of the language server you use for completion.

| Files | Semantic completion | Formatter plugin | Installation example |
| --- | --- | --- | --- |
| JavaScript, TypeScript, JSX, TSX | `lsp-typescript` | **Prettier Formatter** | `npm install -g prettier` |
| Vue, HTML, CSS, SCSS, Less | `lsp-vue`, `lsp-html` or `lsp-css` | **Prettier Formatter** | `npm install -g prettier` |
| JSON, JSONC, YAML, Markdown | Corresponding language preset, where the server offers completion | **Prettier Formatter** | `npm install -g prettier` |
| Python, `.pyi` stubs | `lsp-python` (Pyright) | **Ruff Formatter** | `uv tool install ruff` |
| C, C++, headers | `lsp-clangd` | **ClangFormat Formatter**, or clangd's own formatter | Install `clang-format` from LLVM |

The presets run `prettier`, `ruff` and `clang-format` from `PATH`. To use a project installation, set the formatter’s
**command** to an executable such as `./node_modules/.bin/prettier` or `./.venv/bin/ruff`. Relative paths with a
directory component resolve from the open workspace, or from the file's folder for a standalone file. For example,
after `npm install -D prettier` in a project, use this in your FiveLeaf user settings.

```jsonc
{
  "plugins": { "format-prettier": { "enabled": true, "command": "./node_modules/.bin/prettier" } },
  "formatOnSave": { "typescript": true, "typescriptreact": true }
}
```

Each command receives the unsaved text and the file's path so it can find the project's style configuration.

Open a formatter's detail page in **Settings → Plugins** to change its style for the next format or save.

| Formatter | Personal settings | How project settings apply |
| --- | --- | --- |
| Prettier | `printWidth`, `singleQuote`, `semi` | Prettier uses these when it finds no Prettier config for the file. |
| Ruff | `lineLength`, `quoteStyle` (`single`, `double`, `preserve`) | Each value you set overrides that option in the project's Ruff configuration. Reset it to follow the project again. |
| ClangFormat | `fallbackStyle`, including LLVM, Google and Microsoft | The chosen style applies when clang-format finds no `.clang-format` file. |

For example, this user configuration sets a Python line length and leaves quote style to the project.

```jsonc
{
  "plugins": { "format-ruff": { "enabled": true, "lineLength": 100 } }
}
```

The JavaScript and TypeScript preset sends member requests to the semantic server from the first file you open.
Open the folder containing `jsconfig.json` or `tsconfig.json` to use the project's module resolution rules.
Rust projects can use `lsp-rust` with `rust-analyzer` and `rustfmt` (`rustup component add rust-analyzer rustfmt`).
For other languages, **Format Document** uses the enabled language server when it advertises document formatting.

## Included presets

The install commands below are examples, not commands FiveLeaf runs automatically. Use a compatible version of each
language's runtime. Commands must be visible to the running app's `PATH`; a program available only through an
interactive shell alias is not an executable command. Restart FiveLeaf after changing its launch environment.

| Plugin | Files / language IDs | Server command | Installation example |
| --- | --- | --- | --- |
| `lsp-typescript` | JS, TS, JSX, TSX; `javascript`, `typescript`, `javascriptreact`, `typescriptreact` | `typescript-language-server --stdio` | `npm install -g typescript typescript-language-server` |
| `lsp-html` | HTML | `vscode-html-language-server --stdio` | `npm install -g vscode-langservers-extracted` |
| `lsp-css` | CSS, SCSS, Less | `vscode-css-language-server --stdio` | `npm install -g vscode-langservers-extracted` |
| `lsp-vue` | Vue single-file components | `vue-language-server --stdio` | See the pinned standalone setup below |
| `lsp-svelte` | Svelte components | `svelteserver --stdio` | `npm install -g svelte-language-server` |
| `lsp-go` | Go, `go.mod`, `go.work`; `go`, `gomod`, `gowork` | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| `lsp-python` | Python and `.pyi` stubs | `pyright-langserver --stdio` | `npm install -g pyright` |
| `lsp-rust` | Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| `lsp-clangd` | C and C++; `c`, `cpp` | `clangd` | Install LLVM/clangd from your OS package manager |
| `lsp-java` | Java | `jdtls` | macOS: `brew install jdtls`; elsewhere install the JDT LS launcher and a compatible JDK |
| `lsp-csharp` | C#; `csharp` | `csharp-ls` | `dotnet tool install --global csharp-ls` |
| `lsp-php` | PHP | `intelephense --stdio` | `npm install -g intelephense` |
| `lsp-ruby` | Ruby, Gemfile, Rakefile, gemspecs | `ruby-lsp` | `gem install ruby-lsp` |
| `lsp-lua` | Lua | `lua-language-server` | macOS: `brew install lua-language-server`; elsewhere use the LuaLS release for your OS |
| `lsp-bash` | Bash and `.sh`; `shellscript` | `bash-language-server start` | `npm install -g bash-language-server` |
| `lsp-json` | JSON, JSONC, `tsconfig.json`, `jsconfig.json` | `vscode-json-language-server --stdio` | `npm install -g vscode-langservers-extracted` |
| `lsp-yaml` | YAML | `yaml-language-server --stdio` | `npm install -g yaml-language-server` |
| `lsp-toml` | TOML and common TOML lockfiles | `taplo lsp stdio` | `cargo install --locked taplo-cli --features lsp` |
| `lsp-docker` | Dockerfile, Containerfile, `.dockerfile`; `dockerfile` | `docker-langserver --stdio` | `npm install -g dockerfile-language-server-nodejs` |
| `lsp-terraform` | `.tf`, `.tfvars`; `terraform`, `terraform-vars` | `terraform-ls serve` | Install `terraform-ls` from HashiCorp's packages or releases |
| `lsp-markdown` | Markdown | `marksman server` | macOS: `brew install marksman`; elsewhere use the Marksman release for your OS |

`gopls` normally installs under `$(go env GOPATH)/bin`, Rust tools under `~/.cargo/bin`, and .NET global tools under
`~/.dotnet/tools`. Add the relevant directories to the environment used to launch the app. On Windows, ensure the
server launcher is executable by the OS; if a package exposes only a shell script, supply a wrapper or a custom
declaration that invokes its runtime directly.

The C/C++ preset assigns `.h` to C and `.hpp`, `.hh`, `.hxx` and `.h++` to C++. Clangd uses your project's
`compile_commands.json` for compiler flags and header context. Python interpreter selection belongs in
`pyrightconfig.json` or `pyproject.toml`. Keep Ruby project dependencies in its Gemfile. Open the relevant project root
so servers can find their build and dependency configuration.

## Vue standalone setup

This preset targets **`@vue/language-server@2.2.12`** with `vue.hybridMode` disabled. This version offers a standalone
language server; the preset does not implement the TypeScript-server forwarding needed by newer hybrid configurations.
Do not replace it with the latest Vue server without checking that protocol compatibility.

```sh
npm install -g @vue/language-server@2.2.12
# In the Vue workspace, if TypeScript is not already a dependency:
npm install -D typescript
```

The startup resolver uses Node's standard library to locate the workspace's TypeScript `lib` directory. It runs locally
for a local folder and on the SSH host for a remote folder, without downloading anything. If TypeScript is missing, it
reports the install command. For a different SDK or a separately installed Vue 2.2.12 server, set:

```jsonc
{
  "plugins": {
    "lsp-vue": {
      "enabled": true,
      "command": "/path/to/vue-language-server",
      "tsdkPath": "/path/to/typescript/lib"
    }
  }
}
```

`tsdkPath` can also be relative to the workspace root. Both paths refer to the workspace's machine. The TypeScript
preset handles `.ts`, `.tsx`, `.js` and `.jsx`; the Vue preset handles only `.vue`, so they do not compete for the same
document. [Vue 2.2.12's entry point](https://github.com/vuejs/language-tools/blob/v2.2.12/packages/language-server/node.ts)
documents the standalone initialization path used here.

## Enable and troubleshoot

Enable individual plugins in **Settings → Plugins**, or use the existing plugin settings:

```jsonc
{
  "plugins": {
    "lsp-typescript": { "enabled": true },
    "lsp-html": { "enabled": true },
    "lsp-css": { "enabled": true },
    "lsp-go": { "enabled": true },
    "lsp-json": { "enabled": true },
    "lsp-yaml": { "enabled": true }
  }
}
```

Open a workspace folder and a matching file. If the server fails to start, read its error from the language-server
status menu, check the executable and runtime, then use **Restart**. The plugin's log is also available in
**Settings → Plugins**. Stopping a server suppresses automatic starts until it is restarted or the plugin is reloaded.

For SSH workspaces, install the language server, its runtime and project dependencies on the SSH host. The local
machine's `node_modules`, Python environment and executable paths are not copied to the remote machine.

FiveLeaf currently selects one language server per language. Disable the existing preset before enabling a custom
plugin for the same language. Each marketplace server preset has a **command** field in its plugin settings. Enter an executable
name or path on the workspace machine, such as `./node_modules/.bin/typescript-language-server`. The TypeScript
preset also accepts `logLevel` (1 through 4). For clangd, use `compileCommandsDir` to point to a build directory
and `headerInsertion` to control whether completion adds includes. Apply changes to a running server with
**Language Servers → Restart** in the status bar. Use the reset arrow beside any field to restore its preset.

For other launch arguments or custom initialization options, copy the preset into a plugin with a **different name**,
update its server ID to start with that name, and follow [Language servers in the plugin API](PLUGINS.md#language-servers).
Custom setup can use the existing `resolve` protocol.
