<a name="tasks"></a>

# 任务

简体中文 · [English](TASKS.md)

选择 **查看 → 任务 → 运行任务…**，就能搜索可在 FiveLeaf 终端中运行的命令。macOS 上按 `⌥⌘T`，Windows 和 Linux 上按 `Ctrl+Shift+B`，输入名称后按 `Enter`。用方向键选择任务，按 `Esc` 回到之前使用的编辑器或终端。

任务列表内置 `lazygit` 和 `yazi`。命令面板和 **查看 → 任务** 中也有 **打开 lazygit** 和 **打开 yazi**。请先安装这两个程序，确保 Shell 能找到它们。**打开 yazi** 会进入当前本地文件所在的文件夹，并高亮该文件。没有可定位的文件时，则打开项目文件夹或主目录。

如果启动器传入 `TERM=dumb` 和 `NO_COLOR`，FiveLeaf 会为本地终端启用颜色。若希望某项任务以单色输出，在任务定义中将 `env.NO_COLOR` 设为 `"1"`。每个任务都在编辑器标签页中打开，包括通过快捷键或 **重新运行上次任务** 启动的任务。终端面板拥有独立的 Shell，因此使用或关闭其中一处时，另一处仍可继续运行。

任务选择器和配置格式参考了 Zed 的任务功能。[Zed 任务文档](https://zed.dev/docs/tasks)

<a name="configuration"></a>

## 配置

打开 **设置 → 终端 → 任务**，查看当前生效的任务及其来源。选择 **打开全局任务…** 编辑跨项目共用的命令，或选择 **打开项目任务…** 编辑当前本地文件夹的命令。**查看 → 任务** 中也有这两项操作。保存文件并返回设置，列表就会更新。也可以点击 **刷新** 重新加载。

- 全局任务定义在 FiveLeaf 的 `settings.json` 旁边，文件名是 `tasks.json`。
- 项目任务定义在项目文件夹下的 `.fiveleaf/tasks.json`。
- 同名任务优先使用项目定义，其次是全局定义，最后是内置定义。
- 同一个文件中的任务名称必须唯一。文件允许使用注释和尾随逗号。

每次打开任务选择器都会重新读取配置。保存的改动会在下次启动或重新运行时生效。如果某个配置文件有错，选择器会显示错误并提供 **编辑任务** 入口。其他有效文件中的任务及内置任务仍然可用。打开配置文件时会保留已有内容，包括其中无效的 JSON。

```jsonc
[
  { "label": "lazygit", "command": "lazygit" },
  { "label": "yazi", "command": "yazi" },
  {
    "label": "yazi: current file",
    "command": "yazi",
    "args": ["$FIVELEAF_FILE"],
    "cwd": "$FIVELEAF_WORKTREE_ROOT"
  },
  {
    "label": "go test",
    "command": "go test",
    "args": ["./..."],
    "env": { "GOMAXPROCS": "4" },
    "use_new_terminal": false,
    "allow_concurrent_runs": false
  }
]
```

支持的字段如下。

| 字段 | 含义 | 默认值 |
| --- | --- | --- |
| `label` | 任务选择器显示、快捷键引用的名称 | 必填 |
| `command` | 交给 Shell 解释的命令文本 | 必填 |
| `args` | 追加到命令后的独立参数数组 | `[]` |
| `cwd` | 工作目录。相对路径从项目文件夹开始解析，`~/` 从主目录开始解析 | 项目文件夹或主目录 |
| `env` | 要添加或替换的环境变量对象 | `{}` |
| `use_new_terminal` | 上一次运行结束后，是否打开新终端 | `false` |
| `allow_concurrent_runs` | 同一任务运行时，是否允许启动另一实例 | `false` |

FiveLeaf 支持 Zed 任务格式的一个子集，会忽略其余字段。请把配置放在 FiveLeaf 的任务文件中。`.zed/tasks.json` 不会被自动读取。

<a name="variables-and-arguments"></a>

## 变量和参数

`args`、`cwd` 和 `env` 的值支持 `$NAME` 或 `${NAME}`。下表中的名称都可用 `FIVELEAF_` 或 `ZED_` 前缀，例如 `$FIVELEAF_FILE` 和 `$ZED_FILE` 表示同一个变量。

| 后缀 | 值 |
| --- | --- |
| `WORKTREE_ROOT` | 打开的项目文件夹。没有打开项目时为主目录 |
| `FILE` | 当前文件的完整路径 |
| `DIRNAME` | 当前文件所在的文件夹 |
| `FILENAME` | 当前文件的名称 |
| `RELATIVE_FILE` | 当前文件相对于项目文件夹的路径 |
| `ROW` | 光标所在的行，从 1 开始 |
| `COLUMN` | 光标所在的列，从 1 开始 |

文件变量需要有当前文件。`args`、`cwd` 或 `env` 中引用了不可用的文件变量或未定义的环境变量时，任务会在启动前报错。

环境变量的值依据继承的环境和上下文变量展开，同一任务的 `env` 对象内各项之间不递归引用。例如 `"PATH": "$PATH:./bin"` 会在继承的 `PATH` 后追加目录。参数和工作目录随后可以引用这些已经展开的任务环境变量。

变量展开后，每个参数作为一个字面值传递。参数中的空格、引号和 `$(...)` 等文本都按数据处理，替换值只展开一次。`command` 本身是 Shell 代码，由所用的 Shell 按自身语法解释。FiveLeaf 原样传递命令文本。请通过 `args` 传递文件路径，FiveLeaf 会对其中的上下文变量做展开。

```json
{ "label": "show file", "command": "git diff --", "args": ["$FIVELEAF_FILE"] }
```

Shell 环境中也可以使用上下文变量。在命令文本中，POSIX Shell 使用 `"$FIVELEAF_FILE"`，PowerShell 使用 `$env:FIVELEAF_FILE`。

Windows 上运行任务需要 PowerShell 7.3 或更新版本，并让 `PATH` 能找到 `pwsh.exe`。FiveLeaf 使用其 `Standard` 原生参数传递模式，保留空参数和参数中的引号。较旧的版本会在运行任务前停止。Windows PowerShell（`powershell.exe`）和 `cmd.exe` 不在支持范围内。支持的其他 Shell 包括 sh、bash、zsh、ksh 和 fish。

<a name="reuse-and-rerun"></a>

## 终端复用与重新运行

再次启动同一任务时，通常会把焦点移到它正在运行的终端。成功退出后，任务标签页会关闭。任务失败时会保留输出，并提供 **重新运行** 操作。

**重新运行** 和 **重新运行上次任务** 会重新读取已保存的任务定义，同时沿用最初运行时的文件和光标位置。任务仍在运行时，按并发选项决定如何处理。切换项目后，**重新运行上次任务** 会打开当前项目的任务选择器。

终端复用以项目和任务名称为范围。两个选项的组合行为如下。

| `allow_concurrent_runs` | `use_new_terminal` | 有实例正在运行时 | 所有实例都结束后 |
| --- | --- | --- | --- |
| `false` | `false` | 聚焦正在运行的实例 | 复用已结束的终端 |
| `false` | `true` | 聚焦正在运行的实例 | 打开新终端 |
| `true` | `false` | 另开一个终端 | 复用已结束的终端 |
| `true` | `true` | 另开一个终端 | 打开新终端 |

默认的 `yazi` 任务每次都会定位到当前文件，即使上次运行时已经在 yazi 中切到其他位置。自行指定命令或参数的 `yazi` 任务会按你的配置运行。

任务在本地文件夹中运行。关闭任务终端或它所在的窗口会结束进程。FiveLeaf 启动、打开项目或配置文件变化时，任务终端不会自动恢复或运行。

<a name="keyboard-shortcuts"></a>

## 快捷键

打开 **设置 → 终端 → 任务**，找到任务并选择 **设置快捷键**。按下所需的组合键，再按 `Enter` 保存。点击已显示的快捷键可修改它，或选择 **移除按键** 清除绑定。快捷键根据任务名称查找定义，运行当前项目里的同名任务。

在 **键盘快捷方式** 中可以找到所有任务操作。相关命令有 `task.spawn`、`task.rerun`、`task.lazygit`、`task.yazi`、`task.openGlobal` 和 `task.openProject`。要为特定任务设置快捷键，请为 `task.spawn` 提供与任务 `label` 一致的 `task_name`。

```jsonc
[
  { "key": "ctrl+shift+g", "mac": "cmd+alt+g", "command": "task.lazygit" },
  {
    "key": "ctrl+shift+y",
    "mac": "cmd+alt+y",
    "command": "task.spawn",
    "args": { "task_name": "yazi" }
  }
]
```

把这些条目添加到 `settings.json` 旁的 `keybindings.json`。绑定了名称的快捷键会直接启动对应任务。调用 `task.spawn` 时省略参数，则打开任务选择器。
