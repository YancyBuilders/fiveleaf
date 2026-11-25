<a name="language-servers"></a>

# 语言服务器

简体中文 · [English](LANGUAGE-SERVERS.md)

在 **设置 → 插件** 中按名称安装语言服务器插件，或使用 `ff plugin install`，例如 `ff plugin install lsp-go`。在工作区所在的机器上安装相应的语言服务器，然后启用插件。在工作区打开匹配的文件时，FiveLeaf 才会启动对应的服务器。打开其他类型的文件不会为这些语言安装软件包或启动服务器。

在服务器和编辑器均支持的范围内，预设提供诊断、补全、悬停信息、跳转到定义和格式化。状态栏的语言服务器图标会显示启动错误，菜单中的 **重启** 和 **停止** 可以分别操作当前工作区中的单个服务器，也可操作所有服务器。HTML、CSS 和 JSON 的预设会显式启用服务器内置的格式化功能。

输入暂停后，补全建议会出现在光标处。若服务器声明了 `.` 等触发字符，输入这些字符也会触发补全。用 **选择 → 补全单词** 可随时打开可搜索的补全列表。服务器未返回匹配项时，编辑器会用文档中光标附近的单词作为补全建议。**选择 → 格式化文档** 优先使用格式化插件，其次使用语言服务器。Go 和严格的 JSON 也有内置格式化功能。

<a name="choose-a-formatter"></a>

## 选择格式化器

在工作区所在的机器上安装工具，再在 **设置 → 插件** 中安装并启用对应的格式化插件。按 `Shift+Alt+F` 格式化当前文档，或在设置中开启 **保存时格式化**。格式化器可以独立于用于补全的语言服务器启用。

| 文件 | 语义补全 | 格式化插件 | 安装示例 |
| --- | --- | --- | --- |
| JavaScript、TypeScript、JSX、TSX | `lsp-typescript` | **Prettier Formatter** | `npm install -g prettier` |
| Vue、HTML、CSS、SCSS、Less | `lsp-vue`、`lsp-html` 或 `lsp-css` | **Prettier Formatter** | `npm install -g prettier` |
| JSON、JSONC、YAML、Markdown | 对应的语言预设，取决于服务器是否提供补全 | **Prettier Formatter** | `npm install -g prettier` |
| Python、`.pyi` 类型存根 | `lsp-python`（Pyright） | **Ruff Formatter** | `uv tool install ruff` |
| C、C++ 和头文件 | `lsp-clangd` | **ClangFormat Formatter** 或 clangd 自带的格式化器 | 安装 LLVM 提供的 `clang-format` |

预设从 `PATH` 查找并运行 `prettier`、`ruff` 和 `clang-format`。若使用项目内安装的工具，请将格式化器的 **command** 设为相应的可执行文件，例如 `./node_modules/.bin/prettier` 或 `./.venv/bin/ruff`。包含目录部分的相对路径以当前工作区为起点。单独打开文件时，以该文件所在目录为起点。例如，在项目中执行 `npm install -D prettier` 后，可在 FiveLeaf 的用户设置中加入以下配置。

```jsonc
{
  "plugins": { "format-prettier": { "enabled": true, "command": "./node_modules/.bin/prettier" } },
  "formatOnSave": { "typescript": true, "typescriptreact": true }
}
```

命令会收到文件中尚未保存的文本和文件路径，以便找到项目的代码风格配置。

在 **设置 → 插件** 中打开格式化器的详情页，可以修改下次格式化或保存时使用的风格。

| 格式化器 | 个人设置 | 与项目设置的关系 |
| --- | --- | --- |
| Prettier | `printWidth`、`singleQuote`、`semi` | 找不到项目内的 Prettier 配置时，使用这些值。 |
| Ruff | `lineLength`、`quoteStyle`（`single`、`double`、`preserve`） | 每个已设置的值都会覆盖项目 Ruff 配置中的对应选项。重置后重新采用项目设置。 |
| ClangFormat | `fallbackStyle`，包含 LLVM、Google 和 Microsoft 等风格 | clangd 找不到 `.clang-format` 时，使用所选风格。 |

例如，以下用户配置指定了 Python 的行宽，引号风格由项目设置决定。

```jsonc
{
  "plugins": { "format-ruff": { "enabled": true, "lineLength": 100 } }
}
```

打开第一个文件后，JavaScript 和 TypeScript 预设便会向语义服务器发出成员补全请求。打开包含 `jsconfig.json` 或 `tsconfig.json` 的文件夹，以使用项目的模块解析规则。Rust 项目可搭配 `lsp-rust`、`rust-analyzer` 和 `rustfmt` 使用（`rustup component add rust-analyzer rustfmt`）。对于其他语言，已启用的语言服务器声明支持文档格式化时，**格式化文档** 会使用它。

<a name="included-presets"></a>

## 内置预设

请根据需要手动运行下表中的安装示例，并选用兼容的语言运行时版本。正在运行的 FiveLeaf 必须能通过 `PATH` 找到这些命令。只能在交互式 Shell 中使用的别名无法充当可执行命令。修改启动环境后，请重启 FiveLeaf。

| 插件 | 文件或语言 ID | 服务器命令 | 安装示例 |
| --- | --- | --- | --- |
| `lsp-typescript` | JS、TS、JSX、TSX，语言 ID 为 `javascript`、`typescript`、`javascriptreact`、`typescriptreact` | `typescript-language-server --stdio` | `npm install -g typescript typescript-language-server` |
| `lsp-html` | HTML | `vscode-html-language-server --stdio` | `npm install -g vscode-langservers-extracted` |
| `lsp-css` | CSS、SCSS、Less | `vscode-css-language-server --stdio` | `npm install -g vscode-langservers-extracted` |
| `lsp-vue` | Vue 单文件组件 | `vue-language-server --stdio` | 见下方固定版本的独立模式配置 |
| `lsp-svelte` | Svelte 组件 | `svelteserver --stdio` | `npm install -g svelte-language-server` |
| `lsp-go` | Go、`go.mod`、`go.work`，语言 ID 为 `go`、`gomod`、`gowork` | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| `lsp-python` | Python 和 `.pyi` 类型存根 | `pyright-langserver --stdio` | `npm install -g pyright` |
| `lsp-rust` | Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| `lsp-clangd` | C 和 C++，语言 ID 为 `c`、`cpp` | `clangd` | 用操作系统的软件包管理器安装 LLVM 或 clangd |
| `lsp-java` | Java | `jdtls` | macOS 用 `brew install jdtls`，其他系统安装 JDT LS 启动器和兼容的 JDK |
| `lsp-csharp` | C#，语言 ID 为 `csharp` | `csharp-ls` | `dotnet tool install --global csharp-ls` |
| `lsp-php` | PHP | `intelephense --stdio` | `npm install -g intelephense` |
| `lsp-ruby` | Ruby、Gemfile、Rakefile、gemspec 文件 | `ruby-lsp` | `gem install ruby-lsp` |
| `lsp-lua` | Lua | `lua-language-server` | macOS 用 `brew install lua-language-server`，其他系统使用对应的 LuaLS 发行版 |
| `lsp-bash` | Bash 和 `.sh`，语言 ID 为 `shellscript` | `bash-language-server start` | `npm install -g bash-language-server` |
| `lsp-json` | JSON、JSONC、`tsconfig.json`、`jsconfig.json` | `vscode-json-language-server --stdio` | `npm install -g vscode-langservers-extracted` |
| `lsp-yaml` | YAML | `yaml-language-server --stdio` | `npm install -g yaml-language-server` |
| `lsp-toml` | TOML 和常见的 TOML 锁文件 | `taplo lsp stdio` | `cargo install --locked taplo-cli --features lsp` |
| `lsp-docker` | Dockerfile、Containerfile、`.dockerfile`，语言 ID 为 `dockerfile` | `docker-langserver --stdio` | `npm install -g dockerfile-language-server-nodejs` |
| `lsp-terraform` | `.tf`、`.tfvars`，语言 ID 为 `terraform`、`terraform-vars` | `terraform-ls serve` | 使用 HashiCorp 提供的软件包或发行版安装 `terraform-ls` |
| `lsp-markdown` | Markdown | `marksman server` | macOS 用 `brew install marksman`，其他系统使用对应的 Marksman 发行版 |

`gopls` 通常安装在 `$(go env GOPATH)/bin`，Rust 工具通常位于 `~/.cargo/bin`，.NET 全局工具通常位于 `~/.dotnet/tools`。将相关目录加入应用的启动环境。在 Windows 上，请确认操作系统能够执行服务器启动器。如果安装包只提供 Shell 脚本，请提供包装程序，或编写直接调用运行时的自定义声明。

C/C++ 预设将 `.h` 识别为 C，将 `.hpp`、`.hh`、`.hxx` 和 `.h++` 识别为 C++。clangd 从项目的 `compile_commands.json` 读取编译参数和头文件上下文。在 `pyrightconfig.json` 或 `pyproject.toml` 中指定 Python 解释器。Ruby 项目的依赖应记录在 Gemfile 中。请打开相应的项目根目录，让服务器能够找到构建和依赖配置。

<a name="vue-standalone-setup"></a>

## 配置 Vue 独立模式

此预设面向 **`@vue/language-server@2.2.12`**，并关闭 `vue.hybridMode`。这个版本提供独立的语言服务器，而较新的混合模式需要向 TypeScript 服务器转发请求。当前预设仅支持独立模式。换用最新版 Vue 服务器前，请先核对协议兼容性。

```sh
npm install -g @vue/language-server@2.2.12
# In the Vue workspace, if TypeScript is not already a dependency:
npm install -D typescript
```

启动解析器使用 Node 的标准库查找工作区内 TypeScript 的 `lib` 目录。本地文件夹在本机执行解析器，远程文件夹在 SSH 主机执行，整个过程不下载软件包。找不到 TypeScript 时，会提示安装命令。若需指定其他 SDK，或 Vue 2.2.12 安装在独立路径下，可使用以下配置。

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

`tsdkPath` 也可以相对于工作区根目录。两条路径都指向工作区所在的机器。TypeScript 预设处理 `.ts`、`.tsx`、`.js` 和 `.jsx`，Vue 预设只处理 `.vue`，各自负责不同的文档。[Vue 2.2.12 的入口代码](https://github.com/vuejs/language-tools/blob/v2.2.12/packages/language-server/node.ts) 记录了这里采用的独立模式初始化流程。

<a name="enable-and-troubleshoot"></a>

## 启用和排查问题

可以在 **设置 → 插件** 中逐个启用插件，也可使用现有的插件设置。

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

先打开一个工作区文件夹，再打开匹配的文件。服务器启动失败时，从语言服务器状态菜单查看报错，检查可执行文件和运行时，再选择 **重启**。也可以在 **设置 → 插件** 中查看插件日志。手动停止服务器后，只有重启服务器或重新加载插件才会恢复自动启动。

对于 SSH 工作区，请在 SSH 主机上安装语言服务器、运行时和项目依赖。使用远端自己的 `node_modules`、Python 环境和可执行文件路径。

FiveLeaf 当前为每种语言选择一个语言服务器。要为同一种语言启用自定义插件，请先停用现有预设。插件市场中的语言服务器预设都提供 **command** 设置。填入工作区所在机器上的可执行文件名称或路径，例如 `./node_modules/.bin/typescript-language-server`。TypeScript 预设还支持 `logLevel`，取值为 1 至 4。对 clangd，可以用 `compileCommandsDir` 指向构建目录，用 `headerInsertion` 控制补全时是否插入头文件引用。在状态栏选择 **语言服务器 → 重启**，将更改应用到正在运行的服务器。点击字段旁边的重置箭头可恢复预设值。

若需其他启动参数或自定义初始化选项，将预设复制为一个**不同名称**的插件，将服务器 ID 改为以新名称开头，再按[插件 API 的语言服务器说明](PLUGINS.zh-CN.md#language-servers)配置。自定义配置可使用现有的 `resolve` 协议。
