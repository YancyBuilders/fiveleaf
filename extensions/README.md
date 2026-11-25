# FiveLeaf plugins

Choose a [keymap](#keyboard-shortcuts), [agent](#agents), [debug adapter](#debug-adapters), [color theme](#color-themes), [file icon theme](#file-icon-themes), [language server](#language-servers), [formatter](#formatters) or [editor tool](#editor-tools). Each plugin can be installed by name in **Settings → Plugins** or with the FiveLeaf CLI.

The fallback catalog here mirrors `index.json` of [YancyBuilders/fiveleaf-plugins](https://github.com/YancyBuilders/fiveleaf-plugins). Each plugin directory has its own version in `plugin.json`.

## Debug adapters

Install the adapter on the machine that holds the folder, then enable its plugin in **Settings → Plugins**. **Run and Debug** starts it for the active file, and the editor margin toggles breakpoints.

| Plugin | Adapter | Install |
| --- | --- | --- |
| Go Debugger | `dlv dap` | `ff plugin install debug-go` |
| Python Debugger | `debugpy` | `ff plugin install debug-python` |
| C and C++ Debugger | `lldb-dap` | `ff plugin install debug-cpp` |
| Rust Debugger | `lldb-dap` | `ff plugin install debug-rust` |
| JavaScript and TypeScript Debugger | js-debug | `ff plugin install debug-js` |

Java has no standalone DAP server yet: Microsoft's `java-debug` runs inside Eclipse JDT Language Server, so FiveLeaf does not list a Java debugger.

## Toolbox tools

Enable a tool in **Settings → Plugins → Tools**. Type `$` in the command palette to open it.

Timestamp, Base64, JSON and URL use the DevTool pages shipped with FiveLeaf. Their marketplace manifests select the page, and each has its own version. Tools can also provide their own HTML pages, as Case Converter does.

| Plugin | What it does | Install |
| --- | --- | --- |
| [Timestamp](toolbox-timestamp/) | Timestamps, time zones and durations | `ff plugin install toolbox-timestamp` |
| [Base64](toolbox-base64/) | Encode and decode UTF-8 text | `ff plugin install toolbox-base64` |
| [JSON](toolbox-json/) | Format and compact JSON, convert data formats and generate types | `ff plugin install toolbox-json` |
| [URL](toolbox-url/) | Encode and decode URLs and query components | `ff plugin install toolbox-url` |
| [Case Converter](case-converter/) | Upper, lower, title and sentence case | `ff plugin install case-converter` |

## Color themes

| Plugin | Preview | Install |
| --- | --- | --- |
| [Ayu](themes/ayu/) | <img src="themes/ayu/preview.svg" alt="Ayu preview" height="68"> | `ff plugin install ayu` |
| [Blood Moon](themes/blood-moon/) | <img src="themes/blood-moon/preview.svg" alt="Blood Moon preview" height="68"> | `ff plugin install blood-moon` |
| [Claude](themes/claude/) | <img src="themes/claude/preview.svg" alt="Claude preview" height="68"> | `ff plugin install claude` |
| [Cursor](themes/cursor/) | <img src="themes/cursor/preview.svg" alt="Cursor preview" height="68"> | `ff plugin install cursor` |
| [Grok Bot](themes/grok-bot/) | <img src="themes/grok-bot/preview.svg" alt="Grok Bot preview" height="68"> | `ff plugin install grok-bot` |
| [Jack-o'-Lantern](themes/jack-o-lantern/) | <img src="themes/jack-o-lantern/preview.svg" alt="Jack-o'-Lantern preview" height="68"> | `ff plugin install jack-o-lantern` |
| [One Dark Pro](themes/one-dark-pro/) | <img src="themes/one-dark-pro/preview.svg" alt="One Dark Pro preview" height="68"> | `ff plugin install one-dark-pro` |
| [Purple Gold](themes/purple-gold/) | <img src="themes/purple-gold/preview.svg" alt="Purple Gold preview" height="68"> | `ff plugin install purple-gold` |
| [Qoder](themes/qoder/) | <img src="themes/qoder/preview.svg" alt="Qoder preview" height="68"> | `ff plugin install qoder` |
| [The Matrix](themes/the-matrix/) | <img src="themes/the-matrix/preview.svg" alt="The Matrix preview" height="68"> | `ff plugin install the-matrix` |
| [Witch's Brew](themes/witchs-brew/) | <img src="themes/witchs-brew/preview.svg" alt="Witch's Brew preview" height="68"> | `ff plugin install witchs-brew` |

## File icon themes

| Plugin | Preview | Install |
| --- | --- | --- |
| [Ayu Icons](icon-themes/ayu-icons/) | <img src="icon-themes/ayu-icons/preview.svg" alt="Ayu Icons preview" height="42"> | `ff plugin install ayu-icons` |
| [Catppuccin Latte Icons](icon-themes/catppuccin-latte-icons/) | <img src="icon-themes/catppuccin-latte-icons/preview.svg" alt="Catppuccin Latte Icons preview" height="42"> | `ff plugin install catppuccin-latte-icons` |
| [Catppuccin Mocha Icons](icon-themes/catppuccin-mocha-icons/) | <img src="icon-themes/catppuccin-mocha-icons/preview.svg" alt="Catppuccin Mocha Icons preview" height="42"> | `ff plugin install catppuccin-mocha-icons` |
| [FiveLeaf File Icons](icon-themes/fiveleaf-file-icons/) | <img src="icon-themes/fiveleaf-file-icons/preview.svg" alt="FiveLeaf File Icons preview" height="42"> | `ff plugin install fiveleaf-file-icons` |
| [Material Icon Theme](icon-themes/material-icons/) | <img src="icon-themes/material-icons/preview.svg" alt="Material Icon Theme preview" height="42"> | `ff plugin install material-icons` |
| [Seti Icons](icon-themes/seti-icons/) | <img src="icon-themes/seti-icons/preview.svg" alt="Seti Icons preview" height="42"> | `ff plugin install seti-icons` |

## Keyboard shortcuts

| Plugin | Version | Install |
| --- | --- | --- |
| [JetBrains Keymap](keymaps/keymap-jetbrains/) | 1.0.0 | `ff plugin install keymap-jetbrains` |
| [Sublime Text Keymap](keymaps/keymap-sublime/) | 1.0.0 | `ff plugin install keymap-sublime` |
| [VS Code Keymap](keymaps/keymap-vscode/) | 1.0.0 | `ff plugin install keymap-vscode` |

## Agents

| Plugin | Version | Install |
| --- | --- | --- |
| [Claude Agent](agents/agent-claude/) | 1.0.0 | `ff plugin install agent-claude` |
| [Codex Agent](agents/agent-codex/) | 1.1.0 | `ff plugin install agent-codex` |
| [DeepSeek TUI (DSH) Agent](agents/agent-dsh/) | 1.0.0 | `ff plugin install agent-dsh` |
| [Gemini CLI Agent](agents/agent-gemini/) | 1.1.0 | `ff plugin install agent-gemini` |
| [Kimi Code Agent](agents/agent-kimi/) | 1.1.0 | `ff plugin install agent-kimi` |
| [OMP (Oh My Pi) Agent](agents/agent-omp/) | 1.0.0 | `ff plugin install agent-omp` |
| [OpenCode Agent](agents/agent-opencode/) | 1.0.0 | `ff plugin install agent-opencode` |
| [Pi Agent](agents/agent-pi/) | 1.0.0 | `ff plugin install agent-pi` |
| [Qwen Code Agent](agents/agent-qwen/) | 1.1.0 | `ff plugin install agent-qwen` |

## Language servers

| Plugin | Version | Install |
| --- | --- | --- |
| [Bash / Shell Language Server](languages/lsp-bash/) | 1.0.0 | `ff plugin install lsp-bash` |
| [C / C++ Language Server](languages/lsp-clangd/) | 1.0.0 | `ff plugin install lsp-clangd` |
| [C# Language Server](languages/lsp-csharp/) | 1.0.0 | `ff plugin install lsp-csharp` |
| [CSS / SCSS / Less Language Server](languages/lsp-css/) | 1.0.0 | `ff plugin install lsp-css` |
| [Dockerfile Language Server](languages/lsp-docker/) | 1.0.0 | `ff plugin install lsp-docker` |
| [Go Language Server](languages/lsp-go/) | 1.0.0 | `ff plugin install lsp-go` |
| [HTML Language Server](languages/lsp-html/) | 1.0.0 | `ff plugin install lsp-html` |
| [Java Language Server](languages/lsp-java/) | 1.0.0 | `ff plugin install lsp-java` |
| [JSON / JSONC Language Server](languages/lsp-json/) | 1.0.0 | `ff plugin install lsp-json` |
| [Lua Language Server](languages/lsp-lua/) | 1.0.0 | `ff plugin install lsp-lua` |
| [Markdown Language Server](languages/lsp-markdown/) | 1.0.0 | `ff plugin install lsp-markdown` |
| [PHP Language Server](languages/lsp-php/) | 1.0.0 | `ff plugin install lsp-php` |
| [Python Language Server](languages/lsp-python/) | 1.0.0 | `ff plugin install lsp-python` |
| [Ruby Language Server](languages/lsp-ruby/) | 1.0.0 | `ff plugin install lsp-ruby` |
| [Rust Language Server](languages/lsp-rust/) | 1.0.0 | `ff plugin install lsp-rust` |
| [Svelte Language Server](languages/lsp-svelte/) | 1.0.0 | `ff plugin install lsp-svelte` |
| [Terraform Language Server](languages/lsp-terraform/) | 1.0.0 | `ff plugin install lsp-terraform` |
| [TOML Language Server](languages/lsp-toml/) | 1.0.0 | `ff plugin install lsp-toml` |
| [TypeScript / JavaScript Language Server](languages/lsp-typescript/) | 1.0.0 | `ff plugin install lsp-typescript` |
| [Vue Language Server](languages/lsp-vue/) | 1.0.0 | `ff plugin install lsp-vue` |
| [YAML Language Server](languages/lsp-yaml/) | 1.0.0 | `ff plugin install lsp-yaml` |

## Formatters

| Plugin | Version | Install |
| --- | --- | --- |
| [ClangFormat Formatter](formatters/format-clang/) | 1.0.0 | `ff plugin install format-clang` |
| [Prettier Formatter](formatters/format-prettier/) | 1.0.0 | `ff plugin install format-prettier` |
| [Ruff Formatter](formatters/format-ruff/) | 1.0.0 | `ff plugin install format-ruff` |

## Editor tools

| Plugin | Version | Install |
| --- | --- | --- |
| [TODOs](tools/todos/) | 1.0.0 | `ff plugin install todos` |
| [Word Count](tools/word-count/) | 1.0.0 | `ff plugin install word-count` |

## Releasing a plugin

1. Change the plugin in the marketplace repository and raise `version` in its `plugin.json`.
2. Commit the change and tag it `<name>@v<version>`, for example `one-dark-pro@v1.1.0`.
3. Run the marketplace's index build and copy `index.json` here.

A plugin follows its own version and tag; one release does not move every plugin in the repository.

## Local development

Run `ff plugin dev ./extensions/one-dark-pro` from the app checkout to work on One Dark Pro. Each plugin directory contains a manifest, installation instructions and a license. The marketplace reads [index.json](index.json).
