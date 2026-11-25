<a name="settings"></a>

# 设置

简体中文 · [English](SETTINGS.md)

FiveLeaf 将偏好设置和键盘快捷方式分别保存在两个 JSON 文件中。你可以在[设置窗口](#the-settings-window)中修改，也可以用 FiveLeaf 或其他编辑器直接编辑文件。两种方式都会立即在所有窗口生效，无需重启。

<a name="where-the-files-are"></a>

## 文件位置

FiveLeaf 将应用设置写入自己的数据目录。已有的 Mayoi 数据保存在原处。

| 系统 | 文件夹 |
| --- | --- |
| macOS | `~/Library/Application Support/FiveLeaf` |
| Linux | `$XDG_CONFIG_HOME/FiveLeaf`，未设置时为 `~/.config/FiveLeaf` |
| Windows | `%AppData%\FiveLeaf` |

| 文件 | 写入方 | 内容 |
| --- | --- | --- |
| `settings.json` | 你和 FiveLeaf | 字体、主题、自动保存、Vim 等[偏好设置](#settingsjson) |
| `keybindings.json` | 你和 FiveLeaf | VS Code 格式的[键盘快捷方式](#keybindingsjson) |
| `tasks.json` | 你 | 所有项目都能运行的命令，显示在 **终端 → 任务** 中 |
| `state.json` | FiveLeaf | 窗口、标签页、最近打开的文件夹和布局 |
| `window-state.json` | FiveLeaf | 每个窗口的位置和大小 |
| `crash.log` | FiveLeaf | FiveLeaf 的版本和系统信息，崩溃时还包含退出原因和所有堆栈 |
| `crashes/` | FiveLeaf | 按时间保存最近 10 份崩溃日志。询问是否发送后，文件后缀为 `.sent` 或 `.unsent` |

在 macOS 和 Linux 上，通过设置窗口或 `ff settings` 保存时，用户的 `settings.json` 使用 `0600` 权限。

插件、用户脚本及插件存储的数据位于 `~/.fiveleaf`。参见[插件文件的位置](PLUGINS.zh-CN.md#where-they-are)。

若要改用其他目录或沿用 Mayoi 的设置，请在启动 FiveLeaf 时将 `FIVELEAF_DATA_DIR` 指向该目录。未设置此变量时，也可以用 `MAYOI_DATA_DIR`。FiveLeaf 会在这个目录中读写上面列出的全部文件，你可以借此在独立目录里试用一套设置。先创建目录并退出 FiveLeaf，因为已经运行的实例会接管再次启动的请求。在 macOS 上可以这样启动。

```sh
mkdir -p ~/fiveleaf-test
FIVELEAF_DATA_DIR=~/fiveleaf-test /Applications/FiveLeaf.app/Contents/MacOS/FiveLeaf
```

<a name="the-settings-window"></a>

## 设置窗口

在 macOS 上选择 **FiveLeaf → 设置…**（`⌘,`），在 Windows 和 Linux 上选择 **文件 → 设置…**。设置窗口列出每项偏好、用途及其在 `settings.json` 中的键。

侧栏有八组设置，分别是 **通用**、**外观**、**编辑器**、**终端**、**版本控制**、**键盘快捷方式**、**Agent** 和 **插件**。**外观** 有 **主题**、**窗口**、**材质** 和 **布局** 卡片。**编辑器** 下有 **常规**、**字体**、**Markdown** 和 **按键模式**（Vim 和 Helix）标签页。**终端** 下有 **通用** 和 **任务** 标签页。**任务** 列出生效的任务定义，可以为每项任务录入快捷键，也可以在编辑器里打开全局或项目配置。**版本控制** 下有 **Git** 和 **差异与合并** 标签页。参见[任务配置](TASKS.zh-CN.md#configuration)。

- 顶部的 **搜索设置** 可以跨分类按名称、说明或键搜索。
- 圆点标记已偏离默认值的设置。点击旁边的箭头可以重置。
- 改动会立即应用到所有窗口，并写入 `settings.json`。
- **键盘快捷方式** 列出所有命令及其按键。参见[更改快捷方式](#changing-shortcuts)。
- 打开文件夹后，可以用右上角菜单切换用户设置和该文件夹的设置。参见[文件夹设置](#folder-settings)。
- 侧栏底部的 `settings.json` 和 `keybindings.json` 会在最前面的窗口中打开对应文件。

部分菜单项会打开设置窗口，并直接定位到相关设置。

<a name="settingsjson"></a>

## settings.json

选择 **文件 → 打开设置 (JSON)**，在标签页中打开 `settings.json`。下面的键都可以省略，省略时使用默认值。

| 键 | 默认值 | 可选值 | 其他设置入口 |
| --- | --- | --- | --- |
| `zoomLevel` | `0` | -5 到 8，支持小数 | **查看 → 放大 / 缩小 / 重置缩放**，另见[快捷键](#window-zoom) |
| `fontFamily` | `"monospace"` | 用逗号分隔的字体名称 | |
| `fontSize` | `13` | 9 到 28 | 命令面板中的 **增大编辑器字号 / 减小编辑器字号 / 重置编辑器字号** |
| `fontWeight` | `400` | 100（极细）到 900（极粗） | |
| `fontFeatures` | 无 | OpenType 特性列表 | |
| `lineHeight` | `1.55` | 1 到 3，以 `fontSize` 的倍数表示 | |
| `terminalFontFamily` | `""` | 用逗号分隔的字体名称 | |
| `terminalFontSize` | `0` | 0，或 9 到 28 | |
| `terminalCopyOnSelect` | `true` | `true`、`false` | |
| `theme` | `""` | `""`（FiveLeaf）、`"matrix"`、`"claude"`、`"qoder"`、`"grokbot"`、`"cursor"`、`"onedarkpro"`、`"ayu"`、`"purplegold"`、`"pumpkin"`、`"witchsbrew"`、`"bloodmoon"`，或来自 `themes`、`~/.fiveleaf/themes` 和插件的主题 ID | **查看 → 主题**、**颜色主题…** |
| `iconTheme` | `""` | `""`（FiveLeaf）、`"catppuccin-latte"`、`"catppuccin-mocha"`、`"material"`、`"seti"`、`"ayu"`，或插件提供的图标主题 ID | **设置 → 外观 → 主题 → 文件图标** |
| `themes` | 无 | 按主题 ID 编写的主题配置，格式与主题文件相同 | |
| `appearance` | `""` | `""`（跟随系统）、`"light"`、`"dark"` | **查看 → 主题** |
| `windowOpacity` | `1` | 0.35 到 1，表示背景不透明度 | **设置 → 外观 → 窗口 → 背景不透明度** |
| `micaEnabled` | `false` | `true`、`false` | **设置 → 外观 → 材质 → 云母材质** |
| `micaAnimation` | `true` | `true`、`false` | **设置 → 外观 → 材质 → 动态云母背景** |
| `micaFrost` | `0.55` | 0.15 到 0.95，表示主题表面色的占比 | **设置 → 外观 → 材质 → 云母磨砂浓度** |
| `micaHue` | `177` | 0 到 360 度 | **设置 → 外观 → 材质 → 云母色相** |
| `micaIntensity` | `0.8` | 0 到 1，表示背景光影强度 | **设置 → 外观 → 材质 → 云母光影强度** |
| `micaSpeed` | `1` | 0.25 到 3，以流动速度的倍数表示 | **设置 → 外观 → 材质 → 云母流动速度** |
| `sidebarRight` | `false` | `true`、`false` | **查看 → 侧边栏位于右侧**、**设置 → 外观 → 布局 → 侧栏在右侧**、侧栏按钮的右键菜单 |
| `hideToolboxButton` | `false` | `true`、`false` | **设置 → 外观 → 布局 → 隐藏工具箱按钮** |
| `terminalButtonRight` | `true` | `true`、`false` | **设置 → 外观 → 布局 → 终端按钮在右侧**、终端按钮的右键菜单 |
| `panelPosition` | `""` | `""`（底部）、`"left"`、`"right"` | **查看 → 面板位置** |
| `terminalLocation` | `""` | `""`（面板）、`"editor"` | **查看 → 新建终端在编辑器区域打开** |
| `agentSessionsClaudeDirs` | 无 | 额外的 Claude Code 配置目录列表 | **设置 → Agent → 更多 Claude Code 文件夹** |
| `projectTabs` | `false` | `true`、`false` | **查看 → 项目标签页** |
| `projectTabStyle` | 无 | 每个文件夹的标签、颜色和图标 | 项目栏中相应方块的右键菜单 |
| `locale` | `""` | `""`（跟随系统）、`"en"`、`"zh-Hans"`、`"ja"` | **查看 → 语言** |
| `autoSave` | `true` | `true`、`false` | **文件 → 自动保存** |
| `quickSuggestions` | `true` | 输入时在光标旁显示代码补全建议 | **设置 → 编辑器 → 代码补全** |
| `wordWrap` | `false` | `true`、`false` | **查看 → 自动换行** |
| `lineNumbers` | `""` | `""`（绝对行号）、`"relative"` | **查看 → 行号**、**设置 → 编辑器 → 行号** |
| `smoothCaret` | `true` | `true`、`false` | **设置 → 编辑器 → 平滑光标** |
| `tabSize` | `4` | `1` 到 `8` | |
| `editorAgentContext` | `false` | `true`、`false` | 仅此文件 |
| `previewMermaid` | `true` | `true`、`false` | 仅此文件 |
| `previewMath` | `true` | `true`、`false` | 仅此文件 |
| `markdownMode` | `"live"` | `"live"`（可编辑的预览）、`""`（源代码） | **查看 → Markdown 打开方式** |
| `vim` | `false` | `true`、`false` | **查看 → Vim 模式** |
| `editorMode` | `""` | `""`（都不使用）、`"vim"`、`"helix"` | **查看 → Vim 模式**、**查看 → Helix 按键** |
| `keymap` | `""` | `""`（FiveLeaf）、`"vscode"`、`"jetbrains"`、`"sublime"`，或插件提供的键位方案 ID | **设置 → 键盘快捷方式 → 键位方案** |
| `vimCtrlKeys` | `false` | `true`、`false` | 仅此文件 |
| `blame` | `true` | `true`、`false` | **Git → 行内追溯** |
| `diffLayout` | `""` | `""`（足够宽时并排）、`"split"`、`"inline"` | 差异视图的 **布局** 按钮 |
| `diffCrossFiles` | `""` | `""`（先提示，再按一次跨文件）、`"always"`、`"never"` | 仅此文件 |
| `diffHighlight` | `""` | `""`（变化的词）、`"line"`、`"char"` | 差异视图的 **高亮** 菜单 |
| `diffWhitespace` | `""` | `""`（精确比较）、`"eol"`、`"all"`、`"blank"` | 差异视图的 **空白** 菜单 |
| `diffContext` | `3` | 每处改动周围显示 `0` 到 `100` 行未改动的内容 | 差异视图的 **上下文** 菜单 |
| `diffSyncScroll` | `true` | `true`、`false` | 差异视图的 **同步滚动** 菜单 |
| `diffAlignedRows` | `true` | `true`、`false` | 差异视图的 **布局** 菜单 |
| `mergeAutoApply` | `true` | `true`、`false` | 合并视图菜单中的 **自动应用** |
| `explorerCompactFolders` | `false` | `true`、`false` | **查看 → 压缩文件夹** |
| `searchTree` | `false` | `true`、`false` | 搜索中的 **平铺列表 / 目录树** |
| `scmTree` | `false` | `true`、`false` | 源代码管理中的 **平铺列表 / 目录树** |
| `crashReports` | `""` | `""`（询问）、`"always"`、`"never"` | FiveLeaf 询问时选择 **不再询问** |
| `plugins` | 无 | 各插件的设置，可用 `"enabled": false` 停用插件 | 设置窗口的 **插件** 页面 |
| `agentServers` | `{}` | 按 ID 配置的 ACP 或 CLI Agent，包含命令、环境、标签和超时 | **设置 → Agent → 已注册的 Agent** |
| `copilot` | `{}` | 用于补全和起草提交说明的已注册 Agent ID，可选模型 ID。空值表示关闭 | **设置 → Agent → Copilot** |
| `copilotInlineSuggestions` | `false` | 输入停顿后提供行内补全 | **设置 → Agent → Copilot 行内补全** |
| `aiSecretFiles` | 无 | 内容不发送给 Copilot 的文件、路径、文件夹或通配模式 | **设置 → Agent → 行内补全和生成提交说明时排除的文件** |
| `agentRolePresets` | `{}` | 按稳定 ID 保存的角色覆盖配置 | **设置 → Agent → 角色列表** |
| `agentRoles` | `{}` | 命名角色及其 Agent、模型和提示词 | **设置 → Agent → 角色列表** |
| `formatOnSave` | 无 | 按语言配置的布尔值。`"*"` 表示手动保存时使用的默认值 | **设置 → 插件 → 格式化 → 保存时格式化** |
| `formatters` | 无 | 同一语言有多个格式化程序时，指定使用哪个 | 仅此文件 |

<a name="language"></a>

### 语言

- **`locale`** 决定 FiveLeaf 的显示语言，可用英语、简体中文（`"zh-Hans"`）或日语（`"ja"`）。留空时，按系统语言偏好顺序选用第一个受支持的语言。如果都不匹配，就使用英语。同一种语言的其他标签也可使用，例如 `"zh"` 或 `"ja-JP"`。菜单、命令面板和所有窗口会同时切换。命令面板也支持按英文名称搜索命令。选择日语时，由 MyGo 自己绘制的部分仍使用英语，例如编辑器自身的菜单和系统对话框的按钮。

<a name="window-zoom"></a>

### 窗口缩放

`zoomLevel` 会缩放窗口中的全部内容，包括项目标签页、侧栏、图标、按钮、编辑器和终端。缩放比例为 `1.2 ^ zoomLevel`，`0` 为 100%，`1` 为 120%，`1.5` 约为 131.5%。负值会缩小界面，也支持 `0.5` 这样的小数。保存 `settings.json` 后，改动会应用到所有窗口，重启后仍然生效。

| 操作 | macOS | Windows / Linux |
| --- | --- | --- |
| 放大（级别 +1） | `⌘=` | `Ctrl+=` |
| 缩小（级别 -1） | `⌘-` | `Ctrl+-` |
| 重置（级别 0） | `⌘0` | `Ctrl+0` |

即使焦点在图片上，这些快捷键也会缩放整个窗口。

图片缩放默认没有快捷键。可以按住 `⌘`（macOS）或 `Ctrl`（Windows/Linux）滚动鼠标滚轮，也可以在命令面板中使用 **放大图片 / 缩小图片 / 图片适应视图**。命令面板也提供独立的字号命令，即 **增大编辑器字号 / 减小编辑器字号 / 重置编辑器字号**。

若要恢复 `⌘=`、`⌘-` 和 `⌘0` 以往的行为，请在 `keybindings.json` 中绑定 `editor.fontZoomIn`、`editor.fontZoomOut`、`editor.fontZoomReset`、`image.zoomIn`、`image.zoomOut` 或 `image.zoomReset`。

<a name="fonts"></a>

### 字体

- **`fontFamily`** 是代码使用的字体。可以按回退顺序列出多个字体，例如 `"Iosevka, monospace"`。
- **`fontWeight`** 从字体已有的字重中选取最接近的一个。
- **`fontFeatures`** 用于开关 OpenType 特性。`["calt=0"]` 关闭大多数编程连字，`["ss01"]` 启用一组样式替换。
- **`terminalFontFamily`** 和 **`terminalFontSize`** 用于终端。未指定字体时，终端使用 `fontFamily`。如果所选字体缺少某些图标，会优先回退到已安装的 Nerd Font，再尝试 `monospace`。字号为 `0` 时，比 `fontSize` 小半点。显式设置的终端字号不会随编辑器字号变化。窗口缩放会同时缩放两种字号，基础值保持原样。终端也使用 `fontWeight` 和 `fontFeatures`。

编辑器用数字 `0` 的宽度作为字符单元的宽度，据此对齐各列、缩进参考线和光标。各字符宽度不一致的字体只能大致对齐。

<a name="the-rest"></a>

### 其他选项

- **`theme`** 用于选择颜色主题。留空时使用内置的 FiveLeaf 主题，由 `appearance` 决定深浅模式。The Matrix、Claude、Qoder、Grok Bot、Cursor、One Dark Pro、Ayu、Purple Gold 和万圣节主题分别有对应的插件。万圣节主题沿用原来的配置值，Jack-o'-Lantern 对应 `"pumpkin"`，Witch's Brew 对应 `"witchsbrew"`，Blood Moon 对应 `"bloodmoon"`。One Dark Pro、The Matrix、Purple Gold 和万圣节主题始终为深色，忽略 `appearance`。`~/.fiveleaf/themes` 中的主题以文件去掉 `.json` 或 `.jsonc` 后缀后的名称为 ID，文件中可设置 `dark` 使主题始终为深色。`themes` 下的主题以它在该对象中的键为 ID。参见[主题文档](THEMES.md)。
- **`iconTheme`** 独立于颜色主题设置文件和文件夹图标。FiveLeaf File Icons、Catppuccin Latte 和 Mocha、Material Icon Theme、Seti、Ayu 都有相应的插件。修改后会立即更新已经打开的窗口。所选图标包不可用时，先显示基础的文件和文件夹图标，图标包恢复可用后再采用已保存的选择。参见[文件图标主题](ICON-THEMES.md)。
- **`windowOpacity`** 将背景不透明度设为 0.35 到 1。值为 1 时背景为实色，调低后可透过磨砂玻璃看到桌面。文字和控件的不透明度保持原样。此设置应用到每个主窗口和设置窗口。系统不提供原生背景材质时，窗口使用不透明背景。
- **`micaEnabled`** 在所选配色上叠加磨砂材质，支持浅色和深色外观，也支持没有原生窗口材质的系统。旧的 `"theme": "mica"` 会迁移为这个开关，配色使用 FiveLeaf 默认主题。
- **`micaAnimation`** 控制背景流动。设为 `false` 后使用静态的磨砂背景。系统启用减少动态效果时也会暂停流动。设置窗口获得焦点时，其他可见窗口中的背景仍会继续流动。
- **`micaFrost`**、**`micaHue`**、**`micaIntensity`** 和 **`micaSpeed`** 分别调节表面色、光影颜色、光影强度和流动速度。磨砂浓度越高，所选主题的表面色越明显。拖动滑块时可以在设置窗口中预览。松开后会保存设置，并应用到所有窗口。
- **设置 → 外观 → 材质 → 导入… / 导出…** 可以保存和恢复可移植的 JSON 配置，其中包含六项云母设置和背景不透明度。导入时沿用当前的颜色主题和其他偏好。关闭云母时也可以导入与导出。无效、不完整或不受支持的文件不会写入设置。
- **`smoothCaret`** 让光标平滑移动，拖尾连续跟随，在光标停下后收拢。鼠标点击、键盘导航和选区移动都可触发近距离或远距离滑动，包括 Vim 和 Helix 带次数的移动，以及按单词、行或文档跳转。源码和实时预览均支持。拖动时选区立即更新，光标和拖尾继续跟向选区端点。设为 `false` 可同时关闭滑动和拖尾。启用减少动态效果、双击、滚动、缩放或失去焦点时，光标会立即定位，省略拖尾。
- **`sidebarRight`** 为 `true` 时，将侧栏和底部按钮放在右侧。为 `false` 时，放在左侧，这是默认位置。右键点击侧栏按钮，选择 **底部左侧** 或 **底部右侧**。**查看 → 侧边栏位于右侧** 和 **设置 → 外观 → 布局 → 侧栏在右侧** 修改的也是这项设置。它应用到所有窗口，并在重启后生效。
- **`hideToolboxButton`** 为 `true` 时隐藏侧栏上的工具箱按钮。命令面板仍可打开各项工具，输入 `$` 可单独列出工具。
- **`terminalButtonRight`** 为 `true` 时，将终端按钮放在右下角，这是默认位置。为 `false` 时，放在左下角。右键点击终端按钮，选择 **底部左侧** 或 **底部右侧**，也可以在 **设置 → 外观 → 布局 → 终端按钮在右侧** 中修改。按钮位置独立于侧栏和终端面板的位置，应用到所有窗口，并在重启后生效。
- **`lineNumbers`** 选择编辑器的绝对行号（`""` 或 `"absolute"`）或相对行号（`"relative"`）。相对行号显示每行与光标所在行的距离，光标所在行显示绝对行号。设置应用到所有窗口，并在重启后生效。
- **`panelPosition`** 指定终端面板的位置。`""` 或 `"bottom"` 表示编辑器下方，`"left"` 或 `"right"` 表示窗口一侧。终端面板与侧栏在同一侧时，终端面板位于更靠外的位置。
- **`terminalLocation`** 决定 **新建终端** 和资源管理器里的 **在终端中打开** 会把窗口的第一个终端放在哪里。`""` 表示面板，`"editor"` 表示编辑器标签页。此后打开的终端跟随已有终端的位置。**切换终端**（`` ⌃` ``）会把原有的 Shell 会话带回面板。任务始终在单独的编辑器标签页中打开。
- **`agentSessionsClaudeDirs`** 为 **Agent 会话** 增加要读取的 Claude Code 配置文件夹，例如 `["~/work/claude"]`。该视图默认读取 `~/.claude`、各个 `~/.claude-*` 文件夹，以及 `CLAUDE_CONFIG_DIR` 指定的文件夹。这项设置保存在各台机器上。
- **`projectTabs`** 将所有文件夹放在同一个窗口，由左侧项目栏的标签页切换。切换此设置会立即合并各文件夹窗口或将它们拆开。
- **`projectTabStyle`** 可按文件夹设置项目栏中的项目方块。标签最多两个字符，用来替代首字母或徽标。颜色可选 `"red"`、`"green"`、`"yellow"`、`"blue"`、`"magenta"` 和 `"cyan"`，例如 `{"/Users/me/Downloads": {"label": "DL", "color": "cyan"}}`。远程文件夹的键为 `/ssh:host/path`。`"icon"` 指定 `settings.json` 旁 `project-icons` 文件夹中的图片，用来替代项目徽标。选择 **选择图标…** 或将图片拖到方块，都会把图片复制到该目录。文件夹路径和图片保存在本机。
- **`terminalCopyOnSelect`** 在松开指针按钮时，将终端中选中的内容复制到剪贴板，也支持全选时复制。修改会应用到已经打开的终端。
- **`autoSave`** 在停止输入片刻后或离开文件时，保存已修改的文件。
- **`wordWrap`** 在编辑器边缘折行。按 `⌥Z` 可以单独切换一个标签页的自动换行，直到这项设置下次变化。
- **`tabSize`** 决定编辑器中一个制表符占多少列，各系统一致。按键插入的内容由文件的缩进方式决定。使用空格缩进的文件会沿用自己的缩进宽度。
- **`editorAgentContext`** 在普通文件中指针所在行旁显示 **+**。点击它后，可以在行内审阅卡片中评论该行或已选行。评论留空时只收集代码。你可以跨文件收集片段，再从编辑器工具栏一起发送。此功能默认关闭，修改后立即生效，待处理条目仍保存在审阅队列中。关闭草稿所在的标签页、另写一条评论或跳转到代码时，草稿内容会保存在队列中。**发送** 会先保存当前草稿，再批量发送。
- **`previewMermaid`** 将 Markdown 中的 `mermaid` 代码块绘制为实时预览中的图表。关闭后显示代码。`.mmd` 或 `.mermaid` 文件自身的预览始终绘制图表。
- **`previewMath`** 将 Markdown 中用 `$` 或 `$$` 包裹的 LaTeX 数学内容渲染为实时预览里的公式。`math` 围栏代码块在实时预览中显示为代码。关闭后显示公式源码。
- **`markdownMode`** 默认为 Markdown 文件打开可编辑的实时预览，包括 `.mdx`。设为 `""` 则打开源码。文件路径旁的 **源代码** 将当前标签页切到 Markdown 源码，**预览** 可切回。`⌥⌘V` 有同样的作用。会话会记住每个标签页的视图。
- **`diffHighlight`** 选择差异视图中按词、按字符或按整行标记变化。高亮只影响显示，改动块的操作仍以整行为单位。
- **`diffContext`** 控制每个差异块周围显示多少行未修改内容，其余行折叠起来。
- **`diffAlignedRows`** 保持并排差异两侧行对齐。只有一侧有内容的行，会在另一侧留出空白。关闭后两侧各自连续排列，并独立滚动，此时 `diffSyncScroll` 的值不影响滚动。
- **`diffSyncScroll`** 在滚动时保持并排差异的两侧对齐。关闭后可分别滚动两侧。
- **`vim`** 让编辑器以 Vim 的普通模式打开。它是旧版 Vim 开关。设置了 `editorMode` 时优先使用 `editorMode`。
- **`editorMode`** 让编辑器以 Vim 或 Helix 的普通模式打开，`""` 表示都不使用。Vim 和 Helix 只能选其一。普通模式下按 `Space` 可在右下角打开 Leader 菜单，Vim 的可视模式也支持。`Ctrl-w` 打开编辑器分栏的窗口菜单。插入模式下 `Space` 用于输入空格。在 `keybindings.json` 中绑定 `go.leaderMenu` 可以另设 Leader 菜单快捷键。参见 [Leader 菜单](FEATURES.zh-CN.md#leader-menu)。
- **`vimCtrlKeys`** 在 Windows 和 Linux 上把 `⌃F`、`⌃B`、`⌃R`、`⌃A` 和 `⌃X` 分配给 Vim 普通模式。启用后，这些键不再触发查找、切换侧边栏、打开最近项目、全选和剪切。macOS 上这些常规命令使用 `⌘`，Vim 始终可接管上述 `⌃` 键组合。
- **`blame`** 在光标所在行的行尾显示最后一次修改者及修改时间。
- **`explorerCompactFolders`** 将每一级都只有一个子文件夹的路径合并为资源管理器的一行，例如 `src/main/java/com/example`。对这一行的操作作用于最末级文件夹。关闭后会分别显示各级文件夹，方便重命名或移动中间任一级。也可以从资源管理器中文件夹标题的右键菜单选择 **压缩文件夹**。`node_modules`、`.venv` 和 `target` 等依赖与构建目录单独显示，展开时才会读取。
- **`scmTree`** 在源代码管理中以文件夹树展示改动，默认按路径列出。
- **`searchTree`** 按文件夹对搜索结果分组，独立于源代码管理的显示方式。
- **`crashReports`** 决定 FiveLeaf 启动时如何处理此前生成的崩溃日志。`""` 会询问是否发送给 Mon，`"always"` 总是发送，`"never"` 从不发送。报告是匿名的，并移除你的主目录路径。其他路径如 `/Volumes/...` 或 `/opt/...` 仍会出现在报告中。询问时可点 **查看报告**，查看将要发送的日志。只有显式设置 `FIVELEAF_MON_URL` 指定服务时才会发送报告。在询问界面按 `Enter` 会选择 **不发送**。

<a name="plugins"></a>

### 插件

- **`plugins`** 按名称保存各插件的设置，键由插件的 `plugin.json` 声明。使用 `"enabled": false` 停用插件。文件夹设置无法设置这一项，避免仓库替用户启用插件。参见[插件文档](PLUGINS.zh-CN.md)。
- **`formatOnSave`** 会在手动保存时格式化文档。设置开关为全部受支持语言指定默认值。使用 `{"go": true}` 可以只为 Go 开启，使用 `{"*": true, "json": false}` 可以为 JSON 以外的受支持语言开启。自动保存直接写入当前文本。格式化优先交给格式化插件，其次交给语言服务器，最后使用内置的 Go 和 JSON 格式化程序。语言服务器启动期间，保存时可以用内置格式化程序。
- **`formatters`** 在同一语言有多个格式化插件时，按格式化程序 ID 或插件名指定其中一个，例如 `{"typescript": "prettier"}`。

```json
{
  "plugins": { "word-count": { "wordsPerMinute": 200 }, "prettier": { "enabled": false } },
  "formatOnSave": { "typescript": true },
  "formatters": { "typescript": "biome" }
}
```

<a name="acp-agents"></a>

### ACP Agent

在 **设置 → Agent** 管理 Agent 注册和角色。**已注册的 Agent** 列出自定义的 Agent，以及已启用插件提供的 Agent。相同 ID 同时有本地和插件定义时，只显示一条，并标记为 **覆盖插件配置**。插件提供的条目带有打开插件的链接。

用 **添加自定义 Agent…** 注册自己的 ACP 或 CLI 程序，可分别填写名称、可执行文件、参数和环境变量。已有定义也用同一个表单编辑。移除后可选择 **撤销**。Agent 标题栏和命令面板里的 **配置 Agent…** 会打开此设置页面。你也可以选择 **在 settings.json 中编辑**。

`agentServers` 将外部 Agent 加入 **Agent** 视图。映射的每个键都是稳定的 ID。`name` 可省略，默认使用该 ID。`command` 指定一个可执行文件，`args` 是它的参数列表，`env` 用于为 FiveLeaf 继承的环境添加变量。ACP 程序在连接时启动，CLI 程序在收到消息时启动。命令直接执行，无需 Shell。将选项放入 `args`。如果 `PATH` 找不到可执行文件，请使用绝对路径。

- `protocol` 默认为 `"acp"`，用于协议连接。一次性执行的命令使用 `"cli"`。
- `workingDirectory` 默认为项目文件夹，相对路径从项目文件夹开始解析。ACP 会话和编辑器的路径仍以项目为准。
- `capabilities` 默认为 `[]`。`"code"`、`"reasoning"` 和 `"search"` 等标签只用于路由。
- `disabled` 默认为 `false`。设为 `true` 会停用这个 Agent、断开连接并停止正在进行的工作。
- `timeoutSeconds` 默认为 `120`，允许 1 到 3600 秒。`0` 表示使用默认值。
- `promptArgument` 默认为 `false`。对于 CLI，设为 `true` 时将提示词作为最后一个参数追加到命令，默认通过标准输入传递。

```json
{
  "agentServers": {
    "opencode": {
      "name": "OpenCode",
      "command": "opencode",
      "args": ["acp"]
    },
    "gemini": {
      "name": "Gemini CLI",
      "command": "npx",
      "args": ["-y", "@google/gemini-cli@0.68.0", "--acp"]
    },
    "claude-cli": {
      "name": "Claude CLI",
      "protocol": "cli",
      "command": "claude",
      "args": ["--print"],
      "capabilities": ["code", "reasoning"],
      "timeoutSeconds": 120
    }
  }
}
```

使用第一个示例前请先安装 OpenCode。Gemini 示例需要 Node.js，首次连接时会下载指定版本的软件包。请先通过相应 Agent 的 CLI 登录，凭据和模型配置由该 Agent 管理。最后一个 CLI 示例需要已经安装并登录的 Claude CLI。Agent 对话目前使用本地项目文件夹。

**导入… / 导出…** 用于交换 JSON 文件。仅含 `agentServers` 映射的文件使用 `version: 1`，还含有 `agentRoles` 自定义角色时使用 `version: 2`。改过内置角色的配置使用 `version: 3`，包含 `agentRolePresets`。有跟随当前 Agent 的命名角色时，导出使用 `version: 4`。导出文件只收录自定义定义与覆盖项。

导出文件以私有权限写入，并包含相关的环境变量值。导入会合并新的 Agent ID 和角色名，并接受等效定义。遇到冲突或无效条目会拒绝整个导入。所有映射一起生效。注册和导入均不会启动程序。

在 **设置 → Agent → 对话设置** 中编辑最近一次聚焦的聊天所用的系统提示词和上下文窗口。圆桌会议使用主持人指定的 Agent 和模型来协调讨论。

这是用户级设置。仓库的 `.fiveleaf/settings.json` 无法添加或替换 Agent 程序。也可以在 **设置 → 插件** 中按名称安装并启用 Agent。[ACP Agent 插件](PLUGINS.zh-CN.md#acp-agent-servers)介绍了使用要求和自定义插件声明。[Agent 聊天和圆桌会议](AGENTS-ACP.zh-CN.md)介绍了路由、上下文和代码审阅。

<a name="copilot"></a>

### Copilot

在 **设置 → Agent → Copilot** 中选择 **配置 Copilot…**，选一个已注册的 Agent 后保存。模型留空时使用 Agent 的默认模型，也可以通过 **加载模型** 从 ACP Agent 提供的列表中选择，或直接输入完整的模型 ID。要为 CLI Agent 选择模型，需要在注册的参数中加入 `{model}`。更换 Agent 会清空模型字段。选择 **关闭** 可停用 Copilot。该配置保存到用户设置中，与聊天或角色所选的模型独立。

```json
{ "copilot": { "agent": "my-agent", "model": "my-model" } }
```

在本地项目中，通过 **选择 → 用 Copilot 补全文本** 请求光标位置的补全。选择建议可将其插入文档，或用来替换选区。

开启 **设置 → Agent → Copilot 行内补全** 后，输入停顿时会把光标附近的代码或文本发给该 Agent。建议显示在光标处。按 `Tab` 插入，按 `Esc` 忽略，也可以点击编辑器中的相应控件。已经接受的建议可以用一次撤销移除。单词补全菜单打开时，键盘操作优先作用于菜单候选项。匹配 `aiSecretFiles` 的文件会排除在自动补全之外。

**Git → 用 Copilot 生成提交说明** 使用同一 Agent 和模型填写提交说明框。提交前可以编辑草稿。

草稿根据已暂存的改动生成，没有暂存内容时使用工作树中的改动。在用户设置中通过 `aiSecretFiles` 排除更多文件，生成提交草稿时会省略这些文件的内容，例如 `["private-notes.txt"]`。

<a name="agent-roles"></a>

### Agent 角色

新建圆桌的邀请列表提供 11 个[内置角色](AGENTS-ACP.zh-CN.md#configure-and-use-roles)。在 **设置 → Agent → 角色列表** 中，每行都会预览当前生效的提示词。编辑条目可查看完整提示词，并选择 Agent 和模型。**新增角色…** 打开空白角色。**复制角色** 打开新名称的草稿，带入原角色当前的 Agent、模型和完整提示词。保存后创建角色。

选择 **跟随当前 Agent** 可让角色不绑定特定 Agent。模型留空时使用 Agent 的默认模型。如果打开的聊天都没有所选 ACP 适配器的模型列表，可用 **加载模型** 查询，也可以自己输入模型 ID。查询模型时不会发送提示词。新的邀请使用该角色保存的 Agent、模型和提示词。

预设行及其编辑窗口中的 **还原默认设置** 会将该预设恢复为跟随当前 Agent、使用 Agent 的默认模型和本地化提示词。**还原所有预设角色的默认设置** 可一次重置所有预设。两项操作均可通过通知撤销。

`agentRolePresets` 只保存改过的项，键为 `explore`、`plan`、`implement`、`test`、`review`、`diagnose`、`document`、`requirements`、`interaction`、`compatibility` 或 `moderate`。例如 `"agentRolePresets": {"review": {"agent": "opencode"}}` 为代码审查者指定 Agent，同时沿用本地化提示词。省略 `prompt` 会使用内置文本，显式设置为 `""` 则不使用角色提示词。同名自定义角色是独立的。

已保存和导入的命名角色显示在同一个列表中。角色名是 `agentRoles` 的键。设置 `"followCurrentAgent": true` 的条目应将 `agent` 和 `model` 留空，实际使用时才确定 Agent。指定了 Agent 的条目也可以指定模型。

```json
{
  "agentRoles": {
    "Code review": {
      "agent": "opencode",
      "model": "",
      "prompt": "Review changes for correctness. Cite specific files and explain each finding."
    }
  }
}
```

在 **圆桌会议 → 邀请参会成员** 中选择角色。即使多个角色使用同一个 Agent，每位成员也会使用各自保存的提示词和模型。

`model` 留空时使用 Agent 的默认模型。要指定模型，请输入该 Agent 支持的完整模型 ID。显式指定模型要求 ACP 适配器声明支持模型选择，或要求 CLI Agent 的参数数组包含 `{model}` 字面占位符，例如 `args: ["--print", "--model", "{model}"]`。FiveLeaf 将占位符替换为单个参数值，直接执行程序。模型不受支持或缺少占位符时，FiveLeaf 会阻止发送并保留草稿。如果 CLI 模板含有 `{model}`，必须为角色显式指定模型。若要沿用 CLI 的默认参数，请使用不含占位符的 Agent 定义。

在空闲的 ACP 单聊里，通过输入框的 **模型** 菜单选择模型。输入框下方的上下文指示器显示适配器上报的 token 用量和容量。适配器尚未上报用量时，显示 **上下文 —**。

角色名称可以包含中文和空格。最多支持 128 个角色，每个角色的提示词最多 64 KiB。绑定的 Agent 缺失或已停用时，角色仍会保留并显示，但无法发送消息。更改角色名称会创建新的对话目标，原会话保留历史，继续发送前需选择可用目标。`agentRoles` 属于用户设置，仓库的 `.fiveleaf/settings.json` 无法覆盖。

<a name="share-roles-and-create-them-from-an-agent"></a>

### 分享角色和通过 Agent 创建角色

在 **设置 → Agent → 角色列表** 中选择 **导出角色…**，会保存 `fiveleaf-roles.json`。文件包含你的命名角色和全部内置预设。每个预设都带有稳定 ID、本地化的 `name` 和 `defaultPrompt`，以及已保存的 `agent`、`model` 或 `prompt` 覆盖项。本地化字段供参考，要修改预设提示词请设置 `prompt`。显式设置 `""` 会清空提示词。**导入角色…** 也可以读取第 1 到 4 版 Agent 配置文件里的角色定义。导出 Agent 注册及其环境变量时，使用单独的 Agent 配置导出入口。

有 Shell 访问权限的 Agent 可以一次创建多个命名角色。`roles` 的键是角色名称。设置 `followCurrentAgent` 可以使用当前可用的 Agent，也可以提供 `agent` ID 和可选的 `model` 来指定 Agent 和模型。

```sh
ff roles import - <<'JSON'
{
  "version": 1,
  "roles": {
    "Keyboard reviewer": {"followCurrentAgent": true, "prompt": "Check focus after each action."},
    "Upgrade tester": {"followCurrentAgent": true, "prompt": "Try the upgrade on a copy of the old profile."}
  }
}
JSON
ff roles export > roles.json
ff roles import roles.json
```

`ff roles list` 输出与 **导出角色…** 相同的角色包。导入成功时，会输出新建的命名角色数和已应用的预设覆盖项数，例如 `{"created":2,"presetsApplied":0}`。再次导入相同定义时，两项计数都为零。如果任何定义有冲突或条目无效，整个批次会被拒绝。每个文件或请求负载限 1 MiB，每份用户配置最多保存 128 个命名角色。

要直接调用本地 JSON API，请保持 FiveLeaf 运行，并连接到 `ff roles socket` 输出的 Unix 套接字。每个连接发送一个以换行符结尾的 JSON 对象，读取响应后再关闭套接字。套接字仅供当前用户访问。`list` 和 `export` 将角色包放在 `data` 中。`import` 接收同样的角色包，并在 `data` 中返回计数。失败时返回 `error` 字符串。下面的请求会创建一个命名角色。

```json
{"roles":{"action":"import","bundle":{"version":1,"roles":{"Release reviewer":{"followCurrentAgent":true,"prompt":"Verify the release notes."}}}}}
```

<a name="example"></a>

### 示例

```json
{
  "fontFamily": "JetBrains Mono, monospace",
  "fontSize": 14,
  "fontFeatures": ["calt=0"],
  "lineHeight": 1.6,
  "terminalFontSize": 12,
  "theme": "claude",
  "vim": true
}
```

<a name="good-to-know"></a>

### 配置文件的读写规则

- **使用标准 JSON。** `settings.json` 不接受注释和尾随逗号。`keybindings.json` 支持注释。
- **FiveLeaf 会按自己的格式重写文件。** 启动后不久、用 `⌘,` 打开设置窗口，以及通过菜单、按钮或缩放修改设置时都会重写。重写会记录所有已识别的键，包括默认值。未识别的键会被移除。
- **解析失败时，FiveLeaf 跳过该文件，不显示提示。** 修复并重新保存前，继续使用已有设置。
- **FiveLeaf 会自动校正超出范围的值。** 例如，`fontSize` 为 40 时会调整为 28，`fontFamily` 留空时会调整为 `"monospace"`。
- **仍然支持旧版文件。** 旧版将主题写在外观设置中，因此 FiveLeaf 会把 `"appearance": "matrix"` 或 `"claude"` 读成对应的 `theme`。

<a name="from-the-command-line"></a>

### 通过命令行设置

使用 [`ff` 命令](FEATURES.zh-CN.md#the-ff-command)读取或修改偏好设置，即使 FiveLeaf 已关闭也可以操作。

```sh
ff settings                         # every preference, its key and its value
ff settings get theme fontSize      # a value a line
ff settings set theme=claude fontSize=16
ff settings unset theme             # its value of a first launch again
ff settings set fontFeatures+=ss01  # one item more of a list
ff settings set projectTabStyle.'<folder>'.label=MG
ff settings --folder set vim=true   # the current folder's own file
```

- **符合 JSON 格式的值按 JSON 解析。** `16`、`true` 和 `["calt=0"]` 分别表示数字、布尔值和列表。其余输入按字符串处理，如 `theme=claude` 和 `fontFamily=Fira Code`。`""` 表示空字符串。
- **通过 `+=` 和 `-=` 逐项修改列表。** `fontFeatures+=calt=0` 只在该项不存在时追加，`fontFeatures-=calt=0` 会移除该项。移除最后一项后，列表被清空，无需重写整个列表。
- **项目栏外观可以逐字段设置。** 比如 `projectTabStyle.'<folder>'.label=MG`，也可以用 `.<folder>` 对应的整个对象设置外观，例如 `{"label": "MG", "color": "cyan"}`。`label` 最多两个显示字符，`color` 可选 `red`、`green`、`yellow`、`blue`、`magenta`、`cyan`。字段留空会删除该字段，所有字段都移除后改用项目原本的外观。文件夹名含点也无需引号，例如 `projectTabStyle.~/src/my.app.label`。从当前目录或 `~` 写出的文件夹路径会解析为完整路径。图标是应用复制到自身目录的图片，可通过项目栏菜单选择。命令行修改外观时会保留已选图标。
- **无效值会让命令退出并说明设置接受哪些值。** 未知键、错误类型、超出范围的数字或不受支持的选项均返回状态码 1，不写入修改。
- **命令只写入修改的设置项。** 其他设置沿用文件中的值。键按名称排序，每个键一行。首次写入文件时会创建目录。
- **运行中的 FiveLeaf 会重新读取设置文件。** 改动会立即应用到当前及其他所有窗口，和在编辑器内保存文件的效果相同。
- **`--folder`（`-f`）操作当前文件夹的 `.fiveleaf/settings.json`。** `ff settings --folder` 列出文件夹可以设置的项。用 `get` 查询文件夹尚未覆盖的设置时会输出 `null`。在项目子文件夹中运行时，会向上查找最近的 `.fiveleaf/settings.json`，即项目读取的文件。

<a name="folder-settings"></a>

### 文件夹设置

没有 `.fiveleaf/settings.json` 时，会在原处使用已有的 `.mayoi/settings.json`。没有 `markdownMode` 时，旧的 `markdownPreview` 开关仍会决定 Markdown 视图。`ff settings --folder unset markdownMode` 会同时清除这两个键。

项目文件夹可以在 `.fiveleaf/settings.json` 中单独配置部分偏好，并在该文件夹的窗口里覆盖用户设置。**文件 → 打开文件夹设置 (JSON)** 会打开该文件，文件不存在时会创建。也可在设置窗口右上角的菜单中选择文件夹，此时仅显示文件夹可以设置的项。点击设置旁的箭头可恢复使用用户设置。

可在文件夹级设置 `autoSave`、`quickSuggestions`、`wordWrap`、`markdownMode`、`previewMermaid`、`previewMath`、`terminalCopyOnSelect`、`diffHighlight`、`diffWhitespace`、`diffContext`、`diffCrossFiles`、`mergeAutoApply`、`blame`、`vim`、`editorMode`、`vimCtrlKeys`、`formatOnSave` 和 `formatters`。

```json
{
  "vim": true,
  "wordWrap": true
}
```

- **FiveLeaf 跳过文件夹级别不支持的键**，并在文件中保留这些键。
- **如果文件夹已覆盖某个设置，修改会写入文件夹的配置。** 例如文件夹设置里有 `vim` 时，**查看 → Vim 模式** 会改文件夹的 `vim`，不会改到用户设置。
- **保存文件后会立即应用到该文件夹的窗口。** 签出操作改动此文件时，也会立即生效。
- **文件夹设置适用于本地文件夹。** 可以将文件夹设置随项目一起提交。

<a name="files-fiveleaf-keeps-itself"></a>

## FiveLeaf 自行维护的文件

FiveLeaf 在运行时写入 `state.json` 和 `window-state.json`。若要重置对应状态，先退出 FiveLeaf，再删除相应的文件。

- **`state.json`** 保存打开的窗口及其文件夹、分栏和标签页，包括各标签页的光标、选区、滚动位置和单独的自动换行设置。它也保存资源管理器中展开的文件夹、最近打开的文件夹及重新打开时要恢复的会话，以及每个窗口的布局，包括侧栏视图、展开状态和宽度，还有终端面板的高度及放在一侧时的宽度。删除文件会清除这些状态。
- **`window-state.json`** 保存每个窗口的位置和大小。

0.5.1 之前的版本将窗口和布局保存在 `settings.json`。FiveLeaf 首次启动时会将这些数据迁移到 `state.json`。

<a name="keybindingsjson"></a>

## keybindings.json

**文件 → 打开键盘快捷方式 (JSON)** 会打开 `settings.json` 旁的 `keybindings.json`。该文件使用 VS Code 格式，支持注释，可以直接粘贴 VS Code 或 Cursor 的键位文件。保存后会立即应用到所有窗口。如果无法解析，FiveLeaf 会继续使用此前加载的设置，直到文件修复。无法识别的按键或条件会在通知中显示。

- **`key`** 表示按键（如 `"cmd+1"`）或按键序列（如 `"ctrl+. g c"`）。修饰键支持 `cmd`、`ctrl`、`alt` 和 `shift`。等待组合键的下一个按键时，状态栏会显示按键序列。按下未绑定的键会结束这一序列而不执行操作，也可以按 `Escape` 取消。
- **`command`** 表示 FiveLeaf 命令，可以用 ID，如 `view.sidebar`，也可以用 VS Code 的名称，如 `workbench.action.toggleSidebarVisibility`。在命令名前加 `-` 可移除该命令的按键。`runCommands` 按顺序执行 `commands`，等待上一条命令中的编辑和 Git 操作结束再继续，遇到错误时停止。`vscode.open` 打开 `args` 指定的文件，支持绝对路径、从 `~` 开始的路径或文件夹内的路径。`type` 将 `args` 中的 `text` 输入到编辑器或终端，`workbench.action.terminal.sendSequence` 则发送到终端。触发 FiveLeaf 未提供的命令时会显示提示。
- **`when`** 决定绑定的生效条件。支持 `editorTextFocus`、`editorFocus`、`inputFocus`、`textInputFocus`、`filesExplorerFocus`、`terminalFocus`、`terminalHasBeenCreated`、`terminalIsOpen`、`sideBarVisible`、`editorHasSelection`、`isMac`、`isLinux` 和 `isWindows`，并可用 `!`、`&&`、`||` 和括号组合。其他名称的条件结果为假。

文件中的绑定优先于 FiveLeaf 自带的快捷键。菜单会腾出文件已绑定的按键，`⌘1` 到 `⌘9` 与 `⌃R` 也以文件中的绑定为准。命令面板显示文件中的按键。快捷键在编辑器、终端、资源管理器、差异和图片视图，以及查找、搜索、快速打开和提交说明的输入框中生效。

```json
[
  { "key": "ctrl+. s", "command": "editor.action.indentUsingSpaces" },
  { "key": "ctrl+. g p", "command": "git.push" },
  { "key": "d", "command": "deleteFile", "when": "filesExplorerFocus && !inputFocus" },
  { "key": "cmd+1", "command": "workbench.action.toggleSidebarVisibility" }
]
```

<a name="changing-shortcuts"></a>

### 更改快捷方式

**文件 → 键盘快捷方式…** 会直接打开设置窗口中的命令列表。每条命令都会显示按键与来源。**默认** 表示 FiveLeaf 自带的绑定，**方案** 表示所选键位方案，**用户** 表示 `keybindings.json`，**已移除** 表示文件移除了绑定。**另有 1 个** 表示文件还为该命令配置了带 `when` 或 `args` 的绑定。

- **键位方案** 列出 FiveLeaf 的默认方案和已安装的预设。在 **设置 → 插件** 中安装 **VS Code Keymap**、**JetBrains Keymap** 或 **Sublime Text Keymap**，即可添加该方案或恢复之前保存的选择。按键与 macOS、Windows 或 Linux 系统对应，此处的选择独立于 Vim 和 Helix 编辑模式。插件可以通过 [`contributes.keymaps`](PLUGINS.zh-CN.md)添加更多方案。预设缺失或停用时，会提示并暂时使用 FiveLeaf 的方案，同时保留原 ID 以便插件恢复后继续使用。
- 所选的键位方案提供继承的快捷方式。`keybindings.json` 中的条目优先，切换预设时仍然适用。例如，在 `settings.json` 中设置 `"keymap": "jetbrains"` 可选择 JetBrains 的键位方案。
- **搜索** 可以按命令名称、ID 或快捷键查找。点击旁边的键盘按钮后，可直接按下要查询的按键。
- **点击命令或铅笔图标**，然后按下新的快捷键。录入组合键时按两组键。`Enter` 保存，`Escape` 取消，`Backspace` 重新录入。如果其他命令已使用这些按键，弹窗会列出冲突的命令。
- 点击减号移除该命令继承的按键，点击箭头恢复当前方案中的按键。

FiveLeaf 将每次改动写入 `keybindings.json`，并立即应用。它只修改该命令不带 `when` 或 `args` 的条目，保留注释和原有顺序。新绑定及移除继承绑定的条目会追加在末尾，格式和 VS Code 一样。

```json
{ "key": "cmd+s", "command": "-file.save" },
{ "key": "ctrl+k s", "command": "file.save" }
```

<a name="windows-and-linux-shortcuts"></a>

### Windows 和 Linux 的快捷方式

Windows 和 Linux 通常沿用 macOS 快捷键，将 `⌘` 换成 `Ctrl`，按键提示也会相应变化。下表中的命令采用与 VS Code 一致的其他绑定。

| 命令 | macOS | Windows 和 Linux |
| --- | --- | --- |
| 查找下一个、查找上一个 | `⌘G`、`⇧⌘G` | `F3`、`Shift+F3` |
| 替换 | `⌥⌘F` | `Ctrl+H` |
| 显示第 1 至第 8 个标签页、显示最后一个标签页 | `⌘1` 到 `⌘9` | `Alt+1` 到 `Alt+9` |
| 保存全部、在新窗口中打开文件夹 | `⌥⌘S`、`⌥⌘O` | 无 |
| 聚焦下一栏、聚焦上一栏 | `⌥⌘→`、`⌥⌘←` | 无 |

这两个平台上，`Ctrl` 与 `Alt` 组合用于输入文本，相当于 `AltGr`，所以默认快捷方式不占用这个组合。可以在 `keybindings.json` 中另行绑定这些命令，例如 `{ "key": "ctrl+k s", "command": "workbench.action.files.saveAll" }`。绑定可通过 `isWindows`、`isLinux` 和 `isMac` 区分系统。

<a name="diffwhitespace"></a>

### `diffWhitespace`

控制比较差异时如何处理空白。`""` 表示精确比较，`"eol"` 忽略行尾的空格和制表符，`"all"` 忽略空白，`"blank"` 还将只有空白字符的行视为空行。过滤规则会影响差异的显示和导航。文件文本和改动块操作按原始行处理。
