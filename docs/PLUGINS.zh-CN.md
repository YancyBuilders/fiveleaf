<a name="plugins"></a>

# 插件

简体中文 · [English](PLUGINS.md)

插件可以添加命令、响应编辑器事件、检查和格式化代码、绘制视图和预览，还能提供键位方案、配色主题、图标主题、语言定义、代码片段、语言服务器和 ACP Agent。插件的 JavaScript 或 TypeScript 直接在 FiveLeaf 自带的 [moejs](https://github.com/MoeForge/moejs) 引擎中运行，无需 Node 或构建步骤。WebView 只在插件主动请求时使用。仅声明键位方案、主题或其他静态扩展的插件无需编写代码。

插件 API 尚在完善，1.0 之前还有可能调整。插件通过 `fiveleaf` 声明兼容的 FiveLeaf 版本。现有 Mayoi 插件仍可以导入 `mayoi`、调用 `settings.mayoi()` 和使用 `mayoi` 版本字段。FiveLeaf 用它所基于的 Mayoi 0.20.1 来检查 `mayoi` 字段。两个版本字段都存在时，以 `fiveleaf` 为准。新插件使用 `fiveleaf` 和 `fiveleaf.d.ts`。插件市场和 `ff plugin search` 读取 [FiveLeaf 官方索引](https://raw.githubusercontent.com/YancyBuilders/fiveleaf-plugins/main/index.json)。服务器返回错误时，**设置 → 插件** 会优先使用窗口上次显示的列表，否则使用随 FiveLeaf 提供的列表。选择 **重试** 刷新。`ff plugin search` 在服务中断时也会使用随应用提供的列表。安装列表中的插件仍需访问其 Git 仓库。

<a name="using-plugins"></a>

## 使用插件

<a name="installing"></a>

### 安装

```sh
ff plugin search "One Dark Pro"
ff plugin install one-dark-pro                          # a name from the index
ff plugin install one-dark-pro@1.0.0                    # one version, pinned
ff plugin install YancyBuilders/fiveleaf-plugins#material-icons
ff plugin install https://git.example.com/me/tool.git@v1.2.0
ff plugin list
ff plugin update one-dark-pro                           # omit the name to update all
ff plugin outdated
ff plugin remove material-icons
```

如果 `plugin.json` 位于仓库子目录，使用 `repository#path/to/plugin`。单独填写 Git URL 会选中仓库根目录。在路径后追加 `@ref` 可跟踪分支，或固定在某个标签或提交。本地 Git 路径的用法相同，例如 `./fiveleaf-plugins#material-icons`。每个插件都有自己的安装目录和来源记录。按名称安装时，FiveLeaf 会选择官方索引中当前应用能够运行的最新版本。对于跟随索引的插件，**更新** 和 `ff plugin outdated` 也执行相同的兼容性检查。新版本移到同一仓库的其他子目录时，更新可以随之迁移。在 `@` 后填写版本号会将插件固定到该版本，例如 `one-dark-pro@1.0.0`。旧版安装记录若未保存更新方式，也会保留已安装版本。运行 `ff plugin install <name>` 可从固定版本切换为跟随索引。如果固定版本期间插件来源发生变更，先移除旧的安装，再按名称安装。

直接从 Git 安装的插件跟随原仓库和引用。按名称选择分支、标签或提交时，例如 `one-dark-pro@main`，依然沿用索引记录的子目录。Git 更新独立于索引运行。

打开 **设置 → 插件**，可同时搜索已安装插件和索引。分类筛选可以与 **已安装** 或 **未安装** 搭配使用。停用的插件也列在 **已安装** 中。可从搜索结果安装，或选择 **插件操作 → 从 Git 安装…**。打开已安装插件即可管理其设置。

选择 **安装开发插件…**，再选中包含 `plugin.json` 的文件夹，即可加载本地插件。FiveLeaf 会跟踪该目录中的修改。移除插件时会解除链接，并保留其源文件。

`ff plugin` 直接操作插件目录，因此 FiveLeaf 关闭时也能运行。正在运行的 FiveLeaf 会立即加载插件变更。

插件和设置都需要用户信任。安装插件即信任其代码，并允许它以你的权限运行。请安装你了解的插件。

<a name="where-they-are"></a>

### 存储位置

`FIVELEAF_DATA_DIR` 优先于 `MAYOI_DATA_DIR`。两个变量都未设置时，FiveLeaf 使用 `~/.fiveleaf`。如果 `~/.fiveleaf` 尚不存在而 `~/.mayoi` 已存在，则使用后者。下表使用新的默认路径。

| 目录或文件 | 用途 |
| ----------------------------------- | ----------------------------------------------------------- |
| `~/.fiveleaf/plugins/<name>/` | 已安装的插件，每个插件一个目录 |
| `~/.fiveleaf/plugins/plugins.json` | 每次 Git 安装的来源、版本、提交和更新方式 |
| `~/.fiveleaf/scripts/` | 用户脚本 |
| `~/.fiveleaf/themes/` | 用户自定义主题，见[主题说明](THEMES.md) |
| `~/.fiveleaf/storage/<name>/` | 插件存储的数据 |

在 Windows 上，`~` 代表 `%USERPROFILE%`。设置 `FIVELEAF_DATA_DIR` 后，这些目录位于该变量指定的位置。

安装 **Word Count** 可在状态栏显示阅读时间。安装 **TODOs** 可在侧栏查找 TODO、FIXME 和 HACK 注释。这些工具以及语言服务器、格式化器和 Agent 插件默认停用。安装后，可在 **插件** 页面启用，或在插件设置中指定 `"enabled": true`。见 [ACP Agent](#acp-agent-servers)。

<a name="toolbox"></a>

### 工具箱

在 **设置 → 插件 → 工具** 中启用工具。点击 **Agent 会话** 旁的 **工具箱** 按钮，或在命令面板输入 `$`。按名称或功能搜索，再按 `Enter` 将工具打开为编辑器标签页。再次选中该工具会切回已打开的标签页，并保留输入。在官方工具中，选择 **切换工具** 或按 `Cmd+K`（Windows 和 Linux 用 `Ctrl+K`）即可打开命令面板。点击对应标签页的关闭按钮即可关闭工具。

- **Timestamp** 提供时间戳转换、时区换算和日期计算。
- **Base64** 可将 UTF-8 文本编码为 Base64，也可将 Base64 解码为文本。
- **JSON** 提供格式化、格式转换和类型生成。
- **URL** 提供 URL 和查询参数的编码与解码。

这四个官方插件使用随 FiveLeaf 打包的原版 DevTool Vue 组件，包括功能标签、样式、图标和编辑器。可单独安装和启用每个工具，例如使用 `ff plugin install toolbox-json`。

<a name="vue-and-react-tools"></a>

#### Vue 和 React 工具

工具标签页渲染插件构建后的页面，包括其 DOM、样式表、脚本和其他资源。创建 Vue 项目时，运行以下命令。创建 React 项目时，将第一行改用 `--tool=react`。纯 HTML 工具使用 `--tool`。

```sh
ff plugin new my-tool --tool=vue
cd my-tool
npm install
npm run build
ff plugin dev .
```

编辑 `App.vue` 或 `main.jsx`。运行 `npm run build -- --watch` 可随修改重新构建。发布时包含 `plugin.json` 和完整的 `dist/` 目录，其中应包括样式、字体、图片和按需加载的代码分块。用户可以离线使用已发布的工具。FiveLeaf 以入口 HTML 所在目录为工具源站的根目录提供资源。Vite 模板将构建的基路径设为 `./`。工具内部导航可用基于 hash 的路由，无需服务端提供路由回退。

```jsonc
"contributes": {
  "toolbox": {
    "id": "json.tool",
    "title": "JSON",
    "page": "dist/index.html",            // relative to the plugin's folder
    "features": ["format", "compact"],    // searchable words
    "description": "Format and compact JSON"
  }
}
```

页面可以用 `window.mygo.call("Toolbox.Copy", text)` 复制文字。其余工具箱方法使用相同的调用方式。

| 调用 | 用途 |
| --- | --- |
| `Toolbox.Copy(text)` | 使用编辑器剪贴板复制文本 |
| `Toolbox.Theme()` | 返回 `{ dark, colors }`，提供深色状态和配色 |
| `Toolbox.SetDetail(false)` | 返回工具选择列表 |
| `Toolbox.ManagePlugins()` | 打开 **设置 → 插件 → 工具** |
| `Toolbox.OpenTools()` | 打开主窗口的工具命令面板 |
| `Toolbox.Close()` | 关闭当前工具标签页 |

`window.mygo.on("toolbox-theme", handler)` 会收到后续主题变化。如何应用变化由页面决定。

<a name="when-something-goes-wrong"></a>

### 排查插件问题

- **插件 → 显示插件日志** 在标签页中显示插件的日志和错误，最新记录位于底部。可按插件、级别和关键词筛选。滚动到底部后会跟随新行。**复制** 复制当前显示的行，**清空** 清除日志。最多保留最近 10,000 行。
- **插件 → 显示运行中的插件** 以表格列出插件及其状态、版本、JavaScript 运行时长、内存占用、启动时间、崩溃次数和最近一次错误。点击列标题排序。界面显示时每秒刷新一次。每一行都可重启或停用插件，也可打开详情或日志。
- 命令的 JavaScript 连续运行 10 秒而未让出执行时，FiveLeaf 会将其停止。插件占用超过 256 MiB 时也会停止。连续停止 3 次的插件将被停用，直至选择 **插件 → 重新加载插件**。
- 使用 `ff --disable-plugins` 启动 FiveLeaf，可停用已安装的插件。

<a name="settings-of-plugins"></a>

### 插件设置

```jsonc
{
  "plugins": {
    "word-count": { "enabled": true, "wordsPerMinute": 200 },
    "prettier": { "enabled": false }
  },
  "formatOnSave": { "typescript": true },
  "formatters": { "typescript": "prettier" }
}
```

见[设置指南](SETTINGS.zh-CN.md#plugins)。

<a name="writing-a-plugin"></a>

## 编写插件

```sh
ff plugin new word-stats   # writes ./word-stats
cd word-stats
ff plugin dev              # loads it, and again on each save
```

`ff plugin new` 会生成 `plugin.json`、`main.ts`、供编辑器识别 API 类型的 `fiveleaf.d.ts`，以及为编码 Agent 说明插件用法的 `AGENTS.md`。插件无需构建，每次保存都会重新加载，Agent 写完插件即可检查实际效果。

<a name="a-plugins-folder"></a>

### 插件目录结构

```
word-stats/
  plugin.json
  main.ts
  fiveleaf.d.ts
```

<a name="pluginjson"></a>

### plugin.json

```jsonc
{
  "name": "word-stats",              // lowercase letters, digits and hyphens; the prefix of its ids
  "displayName": "Word Stats",
  "version": "0.1.0",
  "description": "Words of the selection",
  "category": "tools",               // languages, themes, icon-themes, agents, formatters,
                                     // keymaps, snippets, debug-adapters, mcp-servers or tools
  "author": "Ann",                   // optional, shown on the Plugins page
  "fiveleaf": ">=0.1.0",                // the versions of FiveLeaf it works with
  "main": "main.ts",                 // .ts, .js or .mjs
  "activation": ["onCommand:word-stats.count"],
  "enabled": true,                   // optional: false starts it off, until its settings turn it on
  "contributes": {
    "commands": [{ "id": "word-stats.count", "title": "Count Words", "title:zh-Hans": "统计字数" }],
    "keybindings": [{ "key": "ctrl+alt+w", "mac": "cmd+alt+w", "command": "word-stats.count" }],
    "menus": { "plugins": [{ "command": "word-stats.count" }] },
    "settings": { "wordsPerMinute": { "type": "number", "default": 230, "description": "Reading speed" } }
  }
}
```

每个 ID 都以插件名称和一个点开头。可用 `title:<tag>` 提供其他语言的标题，随 **查看 → 语言** 的选择切换。`file`、`edit`、`view` 和 `git` 等 FiveLeaf 自带的前缀为保留前缀。

<a name="activation"></a>

#### 激活条件

插件在它声明的事件发生时启动，在此之前不消耗运行资源。

| 事件 | 触发时机 |
| --------------------------- | ------------------------------------------------------- |
| `onCommand:<id>` | 运行该插件的命令。插件的任一命令均可触发启动。 |
| `onLanguage:<language>` | 对应语言的文档打开或切到前台 |
| `workspaceContains:<glob>` | 文件夹中存在与模式匹配的文件，例如 `go.mod` |
| `onView:<id>` | 显示该插件的一个视图 |
| `onPreview:<id>` | 显示该插件的一个预览 |
| `onStartup` | FiveLeaf 启动 |
| `*` | 上述任一事件 |

语言名称使用语法高亮器识别的小写名称，例如 `go`、`typescript`、`markdown`、`python` 和 `plaintext`，也包括插件声明的语言。

<a name="contributions"></a>

#### 扩展项

| 字段 | 添加的内容 |
| ----------------- | ---------------------------------------------------------------------------------------------- |
| `commands` | 命令面板、快捷键编辑器和 `ff run` 中的命令。`arg` 为 `ff run id=arg` 传入的参数命名。 |
| `keybindings` | 使用 `keybindings.json` 格式的快捷键。插件只能占用空闲的按键，见下文。 |
| `keymaps` | 有名称的快捷键预设，可在 **设置 → 键盘快捷键 → 键位方案** 选择 |
| `menus` | `plugins`（插件菜单）、`explorer/context` 和 `tab/context`。这些菜单的命令会收到路径。 |
| `settings` | 位于 `plugins.<name>` 下的用户设置。支持 `boolean`、`number`、`integer` 和 `string`。用 `enum` 指定选项，用 `minimum` 和 `maximum` 限制整数范围。 |
| `statusItems` | 由插件填充的状态栏条目，`align` 可设为 `left` 或 `right` |
| `views` | 由插件渲染的侧栏视图，与文件浏览器、搜索和源代码管理并列 |
| `previews` | 与匹配 `pattern` 的文件源码并排显示的预览，`output` 可设为 `svg`、`markdown` 或 `view` |
| `formatters` | 适用于指定 `languages` 的格式化器，供 **格式化文档** 和 `formatOnSave` 使用 |
| `webviews` | 侧栏中的 HTML 面板，内容来自插件目录 |
| `themes` | FiveLeaf 或 VS Code 格式的主题 |
| `iconThemes` | 文件和目录图标包，可在 **设置 → 外观 → 文件图标** 选择 |
| `languages` | 文件的 `extensions` 和 `filenames`、用于高亮的 Chroma `lexer`，以及 `lineComment` |
| `snippets` | VS Code 格式的代码片段，适用于指定的 `language` 或 `*` |
| `terminalLinks` | 终端中可点击的链接模式 `pattern`，用于匹配 URL 或路径，并将匹配结果传给 `command` |
| `languageServers` | 为指定 `languages` 提供的语言服务器，包括 `command`、`args` 和 `initializationOptions` |
| `debugAdapters` | 供 **运行和调试** 使用的 DAP 服务器，包括语言 ID、传输方式和启动或附加参数 |
| `toolbox` | 工具箱中的工具，包括 `page`、`title` 和可搜索的 `features` |
| `agents` | 在 **Agent 会话** 中列出和恢复会话的编码 Agent，见下文 |
| `agentServers` | 聊天和圆桌会议使用的 ACP 或 CLI Agent，可声明命令、环境和路由标签，无需 `main` |

<a name="keyboard-shortcuts"></a>

#### 键盘快捷键

只有当按键未被 FiveLeaf 的默认命令或 `keybindings.json` 占用时，插件快捷键才会生效。这样，安装插件不会改变已有按键的用途。发生冲突时，设置窗口的 **插件** 页面会显示该快捷键。选择 **使用插件快捷键** 可将它写入 `keybindings.json`。插件命令可以像 FiveLeaf 自带命令一样，通过 `keybindings.json` 绑定到任意按键。

<a name="keymap-presets"></a>

#### 键位预设

键位插件会在 **设置 → 键盘快捷键 → 键位方案** 添加一套完整的预设。创建名为 `team-keys` 的文件夹，放入以下 `plugin.json`。这个插件无需 `main` 或 JavaScript。

```json
{
  "name": "team-keys",
  "displayName": "Team Keymaps",
  "version": "0.1.0",
  "contributes": {
    "keymaps": [{
      "id": "team-keys.editor",
      "label": "Team Editor",
      "bindings": [
        {
          "key": "ctrl+k ctrl+s",
          "mac": "cmd+k cmd+s",
          "command": "file.keybindings"
        },
        {
          "key": "ctrl+alt+j",
          "mac": "cmd+alt+j",
          "win": "ctrl+shift+j",
          "linux": "ctrl+shift+j",
          "command": "sel.joinLines",
          "when": "editorTextFocus"
        },
        {
          "key": "ctrl+k ctrl+c",
          "mac": "cmd+k cmd+c",
          "command": "runCommands",
          "when": "editorTextFocus && !editorReadonly",
          "args": { "commands": ["sel.comment", "file.save"] }
        }
      ]
    }]
  }
}
```

运行 `ff plugin dev ./team-keys` 加载该文件夹，也可将它放在 `~/.fiveleaf/plugins/team-keys`。Git 仓库和其他插件一样，用 `ff plugin install owner/repo` 安装。在键位方案菜单中选择 **Team Editor**，或在 `settings.json` 中设置 `"keymap": "team-keys.editor"`。

每个预设都需要唯一的 `id`，以插件名称和一个点开头，还需要 `label` 和 `bindings` 列表。`key` 适用于所有系统。分别设置 `mac`、`win`、`linux` 可以覆盖对应系统的快捷键。多段快捷键用空格分隔。`when` 接受 FiveLeaf 的布尔条件，例如 `editorTextFocus`、`terminalFocus`、`!inputFocus`、`&&`、`||` 和括号。`args` 会原样传给命令。命令可以是 FiveLeaf 命令 ID、受支持的 VS Code 别名，或已启用插件的命令。受支持的命令和条件见[键盘快捷键](SETTINGS.zh-CN.md#keybindingsjson)。

选择预设后，键位立即生效。未指定的命令使用 FiveLeaf 默认键位，`keybindings.json` 中的个人设置优先。修改插件会重新加载其预设。停用或移除所选插件时，应用会恢复默认键位，并记住所选 ID，以便重新启用后恢复预设。安装 `keymap-vscode`、`keymap-jetbrains` 或 `keymap-sublime` 可恢复对应的 FiveLeaf 预设。VS Code 和 JetBrains 键位方案的名称表示按键惯例。FiveLeaf 插件使用上面的清单格式。

<a name="maints"></a>

### main.ts

```ts
import { commands, editor, settings, window } from "fiveleaf";

commands.register("word-stats.count", async () => {
  const ed = await editor.active();
  if (!ed) return;
  const s = ed.selection;
  const text = await ed.document.text({ start: s.anchor, end: s.active });
  const words = text.split(/\s+/).filter(Boolean).length;
  await window.notify(`${words} words, ${Math.ceil(words / settings.get<number>("wordsPerMinute"))} min`);
});
```

插件也可以导出 `activate()`。启动时先执行顶层代码，再调用它。

插件代码可以使用以下功能。

- 下文介绍的 `fiveleaf` 模块，其类型见 `fiveleaf.d.ts`。
- `console`，输出写入插件日志。
- `fetch`、`Request`、`Response`、`Headers`、`URL`、`URLSearchParams`、Streams、`AbortController`、`setTimeout`、`setInterval` 和 `queueMicrotask`。
- 用相对路径导入插件自己的文件，可省略扩展名。

插件运行时不提供 `require`、`node:` 模块、npm 包、`eval` 或 `new Function`。要使用 npm 包，请先在插件目录中执行 `npm install`，再运行 `ff plugin pack`。该命令会将导入所需的 ES 模块构建文件复制到 `vendor/`，并将导入路径指向这些文件。插件依赖的包至少需要提供 ES 模块构建。

单个文件的加载上限为 1 MiB。

<a name="the-fiveleaf-module"></a>

### fiveleaf 模块

位置由 `line` 和 `character` 表示，两者均从 0 开始，`character` 按 UTF-16 码元计算，与 JavaScript 字符串及 LSP 一致。所有涉及编辑器的调用都返回 Promise。

| 命名空间 | 调用 |
| ------------------------ | ----------------------------------------------------------------------------------------- |
| `commands` | `register(id, fn, {title})`、`run(id, arg)`（也可运行 FiveLeaf 自带命令）、`list()` |
| `editor` | `active()`、`visible()`。编辑器对象提供 `document`、`selections`、`edit(edits, {version})`、`insert(text)`、`setSelections` 和 `reveal(line)`。 |
| 文档对象 | `path`、`name`、`language`、`version`、`isDirty`、`lineCount`、`text(range?)`、`lineAt(line)`、`save()` |
| `workspace` | `root()`、`isRemote()`、`open(path, {line})`、`readFile`、`writeFile`、`stat`、`findFiles(glob)`、`documents()` |
| `window` | `notify(message, {kind, description, action})`、`fail`、`pick(items)`、`prompt({title, value})`、`progress(title, fn)`、`openText(text)` |
| `settings` | `get(key)` 和 `all()` 获取插件设置，`fiveleaf(key)` 获取 FiveLeaf 设置，`onChange(fn)` 监听变更 |
| `storage` | `get(key)`、`set(key, value)` 和 `keys()`。默认使用全局作用域，传入 `{folder: true}` 可用文件夹作用域。 |
| `events` | `on(name, fn)`，见下文 |
| `process` | `run(command, args, {cwd, input, timeout})`，在文件夹所在机器上执行 |
| `languages` | `setDiagnostics(path, items)`、`clearDiagnostics(path?)`、`registerFormatter(id, fn)`、`registerHover(language, fn)` |
| `statusItem(id)` | `set({text, tooltip, icon, command, onClick})`、`hide()` |
| `views` | `render(id, tree)`、`onShow(id, fn)`、`reveal(id)` |
| `previews` | `register(id, fn)`、`refresh(id)` |
| `webview` | `open(id)`，打开后可用面板的 `post(message)`、`onMessage(fn)` 和 `close()` |
| `agents` | `register(id, fn)`，`fn` 用于列出某个 Agent 的会话 |
| `log` | `info`、`warn`、`error`、`debug` |

提交编辑时要指定计算这些改动所依据的文档版本。期间若文档发生变化，例如用户继续输入，`edit` 会返回 `false` 并跳过修改。成功应用的编辑可一步撤销。

<a name="notifications"></a>

#### 通知

`window.notify()` 在当前编辑器窗口中显示通知卡片，并使用当前主题。可选的 `kind` 决定图标和颜色，取值为 `info`（默认）、`success`、`warning`、`error` 或 `progress`。可选的 `description` 显示在消息下方。`window.fail(message)` 显示错误，并附上插件名称。

```ts
const open = await window.notify("Report ready", {
  kind: "success",
  description: "The workspace report contains 12 findings.",
  action: "Open report",
});
if (open) await workspace.open("report.md");
```

带操作按钮时，用户选择操作后，Promise 返回 `true`。卡片超时或被关闭时返回 `false`。没有操作按钮时，通知一进入队列便返回 `false`。

状态栏始终显示通知铃铛。圆点表示有应用或插件发来的未读通知。铃铛会跨重启保留最近 100 条未读记录，包括超时的卡片。点击条目可标为已读。带圆圈的 × 可移除一条通知，**清空** 可移除全部条目。

<a name="events"></a>

#### 事件

| 事件 | 触发时机 |
| ------------------- | ------------------------------------------------- |
| `editor.open` | 文档打开 |
| `editor.change` | 文档修改后停止编辑 0.3 秒 |
| `editor.focus` | 文档切到前台 |
| `editor.didSave` | 文档手动保存或自动保存 |
| `editor.close` | 文档关闭 |
| `workspace.changed` | 窗口的工作区文件夹改变 |
| `settings.changed` | 设置改变 |
| `theme.changed` | 主题或外观改变 |

文档事件包含文档的 `path`、`language`、`version` 和 `document` 对象。

<a name="files-and-programs-on-ssh-hosts"></a>

### SSH 主机上的文件和程序

插件在本机运行。`workspace` 和 `process` 可以访问文件夹所在的机器。因此 `readFile("go.mod")` 会在远程主机读取文件，`process.run("gofmt", ["-l", "."])` 会在该主机上的工作区文件夹中执行。无需在主机上为插件另装组件。每次调用都会与主机往返一次。

<a name="diagnostics"></a>

### 诊断

```ts
import { events, languages, process } from "fiveleaf";

events.on("editor.didSave", async (e) => {
  if (e.language !== "go") return;
  const { stdout } = await process.run("go", ["vet", "./..."]);
  // ... parse it, then:
  languages.setDiagnostics(e.path, [
    { range: { start: { line: 4, character: 1 }, end: { line: 4, character: 9 } }, severity: "warning", message: "…" },
  ]);
});
```

编辑器会为诊断位置添加下划线，以区分错误、警告、信息和提示。状态栏统计错误和警告的数量，点击可打开 **显示问题** 查看全部诊断。光标所在行的诊断消息会显示在该行旁边。

<a name="formatters"></a>

### 格式化器

使用命令行格式化器时，在清单中声明可执行文件及其参数。FiveLeaf 将当前文档传入命令的标准输入，命令成功退出后应用其 UTF-8 标准输出。命令失败时会显示标准错误。参数直接传给程序，其中 `${file}` 展开为系统原生文件路径，`${tabSize}` 展开为缩进宽度，`${useTabs}` 展开为 `true` 或 `false`。`command` 字段也接受 `${settings.key}`，用来引用插件声明的字符串设置。命令在文件所在机器上运行，工作目录为工作区根目录。文件位于工作区外时，使用该文件所在目录。

用 `settingArgs` 指定可选参数。每个键对应一个已声明的设置，每个值是一条参数模板。FiveLeaf 按键名顺序追加显式设置的参数，包括取值为 `false` 的参数。`${value}` 会原样插入输入值，包括空格。设置未填写、为 `null` 或为空字符串时，会省略整个参数。设置界面使用清单中的默认值。重置字段会移除对应参数，让工具使用自己的配置。

```jsonc
"settings": { "printWidth": { "type": "integer", "minimum": 1 } },
"formatters": [{
  "id": "prettier.format", "languages": ["typescript", "javascript", "css"],
  "command": "prettier", "args": ["--stdin-filepath", "${file}"],
  "settingArgs": { "printWidth": "--print-width=${value}" }
}]
```

使用 JavaScript 格式化器时，声明其 ID 和支持的语言，再在入口模块注册该 ID。`tidy.format` 这样的处理函数可以返回格式化后的文本、文本编辑操作，或用 `null` 表示保留当前文本。

```ts
languages.registerFormatter("tidy.format", async (doc) => (await doc.text()).trimEnd() + "\n");
```

用 **选择 → 格式化文档**（⇧⌥F）即可调用格式化器。为该语言启用 `formatOnSave` 后，手动保存会先尝试格式化，最多等待 1.5 秒。超时后直接保存文件。自动保存时不触发格式化。

<a name="views"></a>

### 视图

视图是一棵节点树，由 FiveLeaf 根据当前主题、缩放级别和语言直接绘制，无需 WebView。

```ts
views.onShow("todo.list", () =>
  views.render("todo.list", {
    type: "list",
    items: todos.map((t) => ({
      key: t.id, label: t.title, description: t.file, icon: "check",
      onClick: () => workspace.open(t.file, { line: t.line }),
      actions: [{ icon: "trash", tooltip: "Remove", onClick: () => remove(t.id) }],
    })),
  })
);
```

可用节点包括带 `children` 的 `stack` 和 `row`，以及 `text`、`markdown`、`code`、`svg`、`list`、`tree`、`input`、`select`、`checkbox`、`button`、`progress`、`divider` 和 `empty`。树条目支持 `children` 和初始展开状态 `open`。`input` 支持 `onChange` 和 `onSubmit`。`progress` 的 `value` 取 0 到 1，省略时显示转动指示器。`empty` 在 `icon` 下方显示 `text`。节点树里的函数会回调到插件，例如 `onClick`。
图标使用 FiveLeaf 自带的名称，包括 `check`、`x`、`plus`、`file`、`folder`、`search`、`star`、`info`、`warning`、`error`、`trash`、`refresh`、`settings`、`terminal`、`branch`、`commit`、`tag`、`clock`、`lightbulb`、`sparkles`、`comment`、`copy`、`external`、`pencil`、`list`、`function`、`hash`、`lock`、`server`、`cloud`、`send`、`keyboard`、`box`、`braces`、`download` 和 `image`。

<a name="previews"></a>

### 预览

```jsonc
"previews": [{ "id": "d2.preview", "pattern": "*.d2", "output": "svg" }]
```

```ts
previews.register("d2.preview", async (text, { path }) => {
  const { stdout } = await process.run("d2", ["-", "-"], { input: text });
  return stdout; // SVG
});
```

用 **查看 → 切换预览**（⇧⌘V）或 **在侧边打开预览** 显示预览。每次编辑后，预览会稍等片刻再重新渲染。输出类型为 `markdown` 时返回 Markdown 文本，为 `view` 时返回节点树。

<a name="webviews"></a>

### WebView

需要绘制节点树之外的内容时，可以让 WebView 在侧栏显示插件目录里的 HTML 文件。

```jsonc
"webviews": [{ "id": "chart.panel", "title": "Chart", "path": "panel.html" }]
```

```ts
const panel = await webview.open("chart.panel");
panel.onMessage((m) => log.info("from the page", m));
panel.post({ points: [1, 2, 3] });
```

页面提供 `fiveleaf.post(message)` 和 `fiveleaf.onMessage(fn)`。在 Linux 上，WebView 需要 WebKitGTK。

<a name="themes"></a>

### 配色主题

```jsonc
"themes": [{ "id": "night.dark", "label": "Night", "path": "night.json" }]
```

`label` 是主题选择器显示的名称，`path` 指向插件目录内的文件。使用绝对路径或指向目录外的路径会导致清单被拒绝。`ff plugin new <name> --theme` 会生成一个无需编写代码的主题插件，可作为起点。

主题文件可以采用 FiveLeaf 格式，以某个 FiveLeaf 配色方案为基础修改颜色。

```json
{
  "base": "dark",
  "colors": { "editor": "#1e1f29", "chrome": "#191a21", "accent": "#bd93f9", "selection": "#44475a80" },
  "syntax": { "keyword": "#ff79c6", "string": "#f1fa8c", "comment": "#6272a4" },
  "ansi": ["#21222c", "#ff5555", "#50fa7b", "#f1fa8c", "#bd93f9", "#ff79c6", "#8be9fd", "#f8f8f2"]
}
```

也可以直接使用 VS Code 配色主题。FiveLeaf 会把其中的 `colors` 和 `tokenColors` 映射到自己的配色。`base` 可设为 `dark`、`light` 或主题 ID，例如 `claude`。主题会出现在 **查看 → 主题 → 颜色主题…**、设置窗口和命令面板。也可直接在 `settings.json` 的 `themes` 下定义主题，无需安装插件。见[主题说明](THEMES.md)。

主题文件还可以设置 `name`、`mono`、`sentence`、`card`、`grain`、`derived` 和 `metrics`，用于调整界面间距、圆角、文字和字重。见[主题说明](THEMES.md)。自定义主题可以直接放进 `~/.fiveleaf/themes`。选择 **插件 → 打开主题文件夹** 即可打开该目录。用户主题与插件主题的 ID 相同时，会显示用户主题。

<a name="icon-themes"></a>

### 图标主题

```jsonc
"iconThemes": [{ "id": "copper-icons.copper", "label": "Copper Icons", "path": "themes/copper.json" }]
```

图标主题可以独立于配色主题更改文件和文件夹图标。其 `id` 以插件名称和一个点开头，`label` 是设置里显示的名称，`path` 指向插件目录内的 JSON 文件。请使用以正斜杠分隔的相对路径。绝对路径、父目录路径片段和指向插件目录外的链接会被拒绝。仅扩展图标的插件无需 `main` 或激活事件。

图标插件的安装、更新、启用和移除方式与其他插件相同，可使用插件命令或同一个设置页面。在 **设置 → 外观 → 文件图标** 中选择图标包。插件停用或移除时，FiveLeaf 会使用默认图标，并保留所选图标包。重新启用或安装该插件即可恢复所选图标。

SVG 图标包格式和完整示例见[图标主题说明](ICON-THEMES.md)。这是 FiveLeaf 自己的格式。Zed 和 VS Code 的图标扩展需先添加 FiveLeaf 清单和映射，之后才能安装。

<a name="languages-and-snippets"></a>

### 语言和代码片段

```jsonc
"languages": [{ "id": "d2", "extensions": [".d2"], "lexer": "yaml", "lineComment": "#" }],
"snippets": [{ "language": "d2", "path": "snippets.json" }]
```

`lexer` 是 [Chroma](https://github.com/alecthomas/chroma) 词法分析器的名称。代码片段采用 VS Code 格式，支持跳转点、占位符、选项和变量，例如 `TM_FILENAME` 和 `CURRENT_YEAR`。用 **选择 → 插入代码片段…** 插入片段。

<a name="language-servers"></a>

### 语言服务器

在 **插件 → 管理插件…** 中安装所需的语言插件，在工作区所在机器上安装各服务器程序，再启用插件。见[语言服务器指南](LANGUAGE-SERVERS.zh-CN.md)。

```jsonc
"languageServers": [{ "id": "gopls.server", "languages": ["go"], "command": "gopls" }]
```

几行插件声明就能接入一种语言。打开匹配的文档时，FiveLeaf 在该文件夹所在机器上启动服务器。对于远程文件夹，服务器在 SSH 主机上运行。FiveLeaf 会显示服务器的诊断，并向其请求 **转到 → 转到定义**（F12）、**转到 → 显示悬停信息**、**选择 → 补全单词**（编辑器内按 ⌃Space），以及在该语言没有格式化插件时请求 **格式化文档**。

直接声明服务器时，`command` 可以用 `${settings.key}` 引用非空字符串，优先使用用户设置，其次使用清单默认值。FiveLeaf 原样传入 `args`。可选参数可使用与格式化器相同的 `settingArgs` 语法。FiveLeaf 在服务器启动和 **重启** 时读取这些设置。

需要准备启动参数的服务器可以使用 `resolve` 代替 `command`。

```jsonc
"settings": { "helperPath": { "type": "string", "default": "my-helper" } },
"languageServers": [{
  "id": "example.server", "languages": ["java"],
  "resolve": { "command": "${settings.helperPath}", "args": ["resolve"] }
}]
```

解析器会在服务器启动时运行一次，执行位置为工作区所在机器。其标准输入是 JSON 对象 `{root, settings}`，包含主机上的绝对路径和插件显式保存的设置，不包含清单默认值。解析器命令和参数中的 `${settings.key}` 引用优先取已保存的设置，其次取清单默认值。替换后的参数直接传给进程，不经过 Shell。解析过程应快速完成并可离线运行，依赖安装应放进单独的配置命令。解析和初始化共用 30 秒的时限。

解析器必须输出一个 JSON 对象并成功退出。

```ts
{
  command: string;
  args?: string[];
  env?: Record<string, string>;
  unsetEnv?: string[];
  initializationOptions?: unknown;
  configuration?: Record<string, unknown>;
  workspaceFolders?: { uri: string; name: string }[];
  pathMappings?: { clientURI: string; serverURI: string }[];
}
```

FiveLeaf 直接启动返回的命令。`env` 覆盖继承的环境变量，`unsetEnv` 在工作区主机上移除对应变量。`configuration` 根据以点分隔的配置节名响应 `workspace/configuration` 请求。`workspaceFolders` 用于初始化和工作区文件夹请求，第一个文件夹提供 `rootUri`。文件夹和映射必须使用绝对 `file:` URI。URI 前缀映射支持双向转换，按完整路径组件匹配，并优先使用最长的前缀。映射会处理已知的 LSP 文档、工作区、定义和诊断 URI 字段，也包括工作区编辑中的键。源码与编辑文本、悬停信息、配置、`initializationOptions` 和未知的扩展载荷会原样传递。插件在自己的配置里填入服务器专用路径。这样服务器可以导入规范路径，编辑器则可在跳转定义和显示诊断时保留工作区里的符号链接别名。标准的服务器声明仍可使用。

<a name="debug-adapters"></a>

### 调试适配器

要试用处理新文件扩展名的调试适配器插件，请生成并链接这个示例。

```sh
ff plugin new sample-debug --debug-adapter
cd sample-debug
ff plugin dev .
```

示例在 `example.sample-debug` 中使用 Python 语法。用 `python -m pip install debugpy` 在 Python 环境中安装 `debugpy`，再在 **设置 → 插件 → Sample debug** 中填入解释器路径。将生成的文件夹作为项目打开。点击编辑器边栏或按 `F9` 切换第 2 行的断点，再按 `F5`，即可在 **运行和调试** 中看到 `message` 的值。已安装插件的详情页在 **调试适配器** 下列出该适配器。

把鼠标悬停在行号左侧可预览断点。点击添加，再点一次移除。调试期间，实心圆点表示已获适配器确认的断点。悬停在空心圆点上可查看验证状态。圆点旁的箭头指示当前暂停的行。

要使用自己的服务器，请在 `plugin.json` 中修改语言关联、可执行文件和启动参数。服务器可以用任意语言实现。下面的示例启动插件内打包的 Node.js 适配器。该适配器实现 [Debug Adapter Protocol](https://microsoft.github.io/debug-adapter-protocol/)。

```json
{
  "name": "acme-debug",
  "displayName": "Acme Debugger",
  "version": "0.1.0",
  "category": "debug-adapters",
  "contributes": {
    "languages": [{ "id": "acme", "extensions": [".acme"] }],
    "debugAdapters": [{
      "id": "acme-debug.server",
      "name": "Acme Debugger",
      "adapterID": "acme",
      "languages": ["acme"],
      "command": "node",
      "args": ["${pluginDir}/adapter.mjs"],
      "launch": { "program": "${file}", "cwd": "${workspaceFolder}" }
    }]
  }
}
```

适配器中的 `languages` 列出它处理的语言 ID。对于 FiveLeaf 已识别的语言，使用现有 ID。对于新语言，按示例在 `contributes.languages` 中声明扩展名或文件名。多个已启用的适配器能处理当前语言时，按 `F5` 会让用户选择。扩展项 `id` 以插件名称和一个点开头。`adapterID` 指定 DAP `initialize` 请求发送的 ID，默认等于扩展项 `id`。如果适配器会读取 `initializationOptions`，插件可以提供该字段。

默认通过标准输入输出通信。适配器从标准输入读取分帧的 DAP 请求，向标准输出写入分帧的消息。使用 TCP 适配器时，将 `transport.kind` 设为 `tcp`，并选择一种获取端口的方法。

| 适配器行为 | 清单字段 |
| --- | --- |
| 输出所选端口 | `"command": "dlv", "args": ["dap"], "transport": { "kind": "tcp", "ready": "DAP server listening at: " }` |
| 已在固定地址监听 | `"transport": { "kind": "tcp", "address": "127.0.0.1:4711" }` |

使用固定地址时，省略 `command` 即可直接连接。插件提供了命令时，FiveLeaf 启动进程，并在整个会话中管理它。发送 DAP `attach` 请求需要另行配置。按 `F5` 时，FiveLeaf 优先使用 `launch` 对象。如果只声明了 `attach` 对象，就使用它。也可在 `launch` 中设置 `"request": "attach"` 选择 `attach` 模式。将目标适配器需要的参数放在选中的对象中。

命令、参数、环境变量的值以及 `launch` 或 `attach` 中的字符串可以用 `${settings.key}` 引用字符串设置，也可以用 `${file}`、`${workspaceFolder}`、`${fileDirname}` 和 `${fileBasename}` 引用路径。`args` 中的每项都作为一个独立的进程参数。对于本地项目，`${pluginDir}` 指向已安装插件的文件。对于 SSH 工作区，请在工作区主机上安装适配器及所有辅助程序，并通过远程 `PATH` 上的名称或主机上的绝对路径引用对应可执行文件。

TCP 适配器可以用标准的 `startDebugging` 反向请求在同一台服务器上打开子目标。FiveLeaf 会发送子目标的配置，将源码断点应用到该目标，并显示当前暂停目标的调用栈和变量。JavaScript 和 TypeScript 插件对 Node.js 使用这种方式。在编辑器里启动程序的适配器使用内置调试控制台。通过 SSH 启动 TCP 进程时，建议用 `ready` 消息通知 FiveLeaf 监听器何时可以接收连接。

<a name="resolving-a-launch-at-runtime"></a>

#### 运行时解析启动配置

`resolve` 辅助程序可以查找针对当前工作区的可执行文件、端口或启动参数。例如，可以在上述适配器声明中添加 `"resolve": { "command": "node", "args": ["${pluginDir}/resolve.mjs"] }`。辅助程序从标准输入读取一个 JSON 对象，其中包含 `root`、`file` 和插件显式保存的 `settings`。它向标准输出写入一个 JSON 对象后退出。例如，以下输出会为会话提供启动参数。

```json
{ "env": { "ACME_MODE": "debug" }, "launch": { "entryPoint": "main" } }
```

省略的字段使用清单中的值。`env` 和 `launch` 按键覆盖。返回的 `command`、`args`、`transport` 和 `initializationOptions` 则会整体替换对应字段。解析器也可以返回完整的适配器配置。请将诊断信息写入标准错误。解析和适配器启动握手共用 30 秒的超时时间。

<a name="coding-agents"></a>

### 编码 Agent

**Agent 会话** 列出 Claude Code、Qoder、Codex 和 Grok 的会话。插件可以添加其他 Agent，会话会带着该 Agent 的图标显示。选择 **恢复** 可在编辑器聊天中打开历史对话。

```jsonc
"agents": [{
  "id": "pi.agent", "name": "Pi",
  "icon": "sparkles",                      // one of the icons of views, or an SVG: "<path d=\"…\"/>" of a 24×24 view
  "tint": ["#e0559b", "#b8326f"],          // optional: one hex color, or one for dark windows and one for light
  "resume": "pi --resume {id}",            // copied by Copy Resume Command, with placeholder values quoted
  "sessions": {                            // optional: a session for each file the glob matches
    "glob": "~/.pi/sessions/**/*.jsonl", "format": "jsonl",
    "id": "sessionId", "title": "title", "folder": "cwd", "updated": "timestamp", "prompt": "message.content"
  }
}]
```

`sessions` 会读取 glob 匹配的每个文件，其中 `~` 表示用户主目录，`**` 匹配任意层级的目录。`json` 文件包含一个对象，`jsonl` 则每行一个对象。每个字段使用以点分隔的键路径，例如 `meta.cwd` 或 `messages.0.content`。读取 `jsonl` 时，通常取首次出现该字段的行，`title` 和 `updated` 则取最后一行。未指定 `id` 字段时，会话 ID 使用去除扩展名的文件名。未指定 `updated` 字段时，使用文件时间。时间可用自 1970 年起的毫秒数、秒数或 ISO 8601 文本表示。文件只有发生变更后才会重新读取。

Agent 未声明 `sessions` 时，可以用函数列出会话。视图开始读取会话时，FiveLeaf 会启动插件并调用该函数。

```ts
import { agents, workspace } from "fiveleaf";

agents.register("pi.agent", async () => {
  const index = JSON.parse(await workspace.readFile("/Users/me/.pi/index.json"));
  return index.map((s) => ({ id: s.id, title: s.name, folder: s.cwd, updated: s.mtime, prompt: s.firstPrompt }));
});
```

每次读取会话时，FiveLeaf 都会在主线程之外调用该函数。视图显示期间每隔几秒调用一次，等待 1.5 秒。之后到达的结果会稍后显示。函数运行到 10 秒会被放弃。调用失败时，应用保留上次取得的会话，并把错误写入插件日志。同一个错误只记录一次，直到错误内容改变。停用插件后，列表会隐藏该插件的 Agent 及其会话。

<a name="acp-agent-servers"></a>

### ACP Agent 服务

**Agent** 界面通过 [Agent Client Protocol](https://agentclientprotocol.com/) 连接 Agent，也可以运行一次性的 CLI 命令。两种 Agent 都能参加独立的圆桌会议。声明 Agent 服务无需 JavaScript 入口文件或会话扫描器。

```json
{
  "name": "my-agent",
  "displayName": "My ACP Agent",
  "enabled": false,
  "contributes": {
    "agentServers": [{
      "id": "my-agent.acp",
      "name": "My Agent",
      "command": "my-agent",
      "args": ["--acp"],
      "env": { "AGENT_PROFILE": "work" },
      "capabilities": ["code", "reasoning"]
    }]
  }
}
```

每个 ID 都必须以插件名称和一个点开头，并在该插件的 Agent 服务声明中唯一。`command` 指定可执行文件，参数单独放在 `args` 中。仅填写命令名时，FiveLeaf 会从 `PATH` 查找。相对路径以插件目录为起点，例如 `./bin/agent`。默认在当前项目文件夹内运行 Agent。`env` 扩充 FiveLeaf 继承的环境。启用插件后即可选择该 Agent。在 **Agent** 中连接会启动 ACP，而 CLI 会在每次发送消息时运行。停用插件会移除该提供方。

使用 `npx` 等启动器时，将 `localCommand` 设为同时必须安装的 Agent CLI。FiveLeaf 在启用插件前会检查两个可执行文件。`localCommandEnv` 可以指定一个保存 Agent 可执行文件路径的环境变量，例如 Codex 使用的 `CODEX_PATH`。

可选字段与[用户级 Agent 设置](SETTINGS.zh-CN.md#acp-agents)一致，包括 `protocol`（默认 `acp`，也可为 `cli`）、`workingDirectory`、`capabilities`、`disabled`、`timeoutSeconds`（默认 120 秒）以及仅 CLI 支持的 `promptArgument`。相对工作目录以项目为起点。ACP 会话和编辑器的文件访问范围仍以原有边界为准。`code`、`reasoning` 和 `search` 等标签用来选择下一位圆桌发言者，本身不授予权限。CLI 默认从标准输入接收提示词。`promptArgument` 为 `true` 时，提示词会作为一个完整的参数追加到命令末尾。命令必须以非交互方式运行，并在回复后退出。

在 **设置 → 插件** 中安装并启用 Agent，再在聊天中选用它。

| 插件 | Agent | 启动命令 |
| --- | --- | --- |
| `agent-pi` | Pi | `npx -y pi-acp@0.0.34` |
| `agent-omp` | OMP (Oh My Pi) | `omp acp` |
| `agent-dsh` | DeepSeek TUI (DSH) | `dsh --profile acp` |
| `agent-codex` | Codex | `npx -y @agentclientprotocol/codex-acp@2.7.0` |
| `agent-claude` | Claude Code | `npx -y @agentclientprotocol/claude-agent-acp@0.107.0` |
| `agent-gemini` | Gemini CLI | `gemini --acp` |
| `agent-qwen` | Qwen Code | `qwen --acp` |
| `agent-opencode` | OpenCode | `opencode acp` |
| `agent-kimi` | Kimi Code | `kimi acp` |

`npx` 预设需要 Node.js，首次连接时会下载固定版本的软件包。其他插件使用已安装 Agent 原生提供的 ACP 命令。请在连接前通过其 CLI 登录。运行时和账号要求见 [Agent 配置指南](AGENTS-ACP.zh-CN.md#agent-plugins)。

用户可以在 Agent 设置中覆盖插件的 Agent 定义，包括标签和启用状态。

若要使用本地可执行文件或其他 ACP 或 CLI Agent，可选 **配置 Agent… → 添加自定义 Agent…**，或使用用户级 [`agentServers` 设置](SETTINGS.zh-CN.md#acp-agents)。原有的 `contributes.agents` 声明继续支持在 **Agent 会话** 中展示，以及 **复制恢复命令**。一个插件可以同时提供这两类扩展。

<a name="user-scripts"></a>

### 用户脚本

`~/.fiveleaf/scripts` 中的 `.ts` 或 `.js` 文件会作为无需清单的插件运行。它导出的每个函数都成为 `script.<file>.<function>` 格式的命令，标题取自函数名称，可在命令面板、快捷键编辑器和 `ff run` 中使用。用 `ff run script.text.upperCase=hello` 可以向函数传入 `hello`，从命令面板调用则不传参数。选择 **插件 → 打开脚本文件夹** 会创建目录并放入示例。

选择 **插件 → 将选中内容作为脚本运行**，会把选区作为独立插件运行一次。没有选区时运行整个文件。`console.log` 写入日志，默认导出的返回值会显示在通知中。

<a name="sharing-a-plugin"></a>

## 分享插件

将 `plugin.json` 放到仓库根目录或插件子目录。所选目录及其中的文件都应避免使用符号链接。索引条目的 `name` 应与清单中的 `name` 一致。向 [YancyBuilders/fiveleaf-plugins](https://github.com/YancyBuilders/fiveleaf-plugins) 提交 Pull Request，将条目加入 `index.json`。

```json
{ "name": "material-icons", "displayName": "Material Icon Theme", "repository": "https://github.com/YancyBuilders/fiveleaf-plugins.git#material-icons", "description": "Material Icon Theme artwork for files and folders in FiveLeaf.", "tags": ["icon-themes"] }
```

发布官方版本时，将 `extensions/` 中的插件目录、许可证、展示用 README 和 `index.json` 发布到插件仓库 `main` 分支的根目录。Go 编写的目录加载器和测试由 FiveLeaf 客户端维护。原先使用 `fiveleaf-themes` 或 `fiveleaf-icons` 的用户可以先单独安装需要的插件，再移除旧插件包。FiveLeaf 会识别已保存的选择。

`language`、`themes`、`icon-themes`、`lsp`、`agent`、`snippet`、`formatter` 和 `keymap` 等标签会将索引条目归入各自的目录筛选项。已安装插件根据各自清单里的扩展项分类。**语言** 筛选项包括带 `language` 和 `lsp` 标签的条目，也包括声明其中任一扩展项的已安装插件。当索引含有 `debug-adapter` 或 `mcp-server` 标签，或已安装插件声明了相应扩展项时，会出现 **调试适配器** 和 **MCP 服务** 筛选项。
