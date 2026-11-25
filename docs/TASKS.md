# Tasks

**View → Tasks → Run Task…** opens a searchable list of commands to run in FiveLeaf's terminal.
Use `⌥⌘T` on macOS or `Ctrl+Shift+B` on Windows and Linux, type a name, then press `Enter`.
The arrow keys choose a task; `Esc` returns to the editor or terminal you were using.

`lazygit` and `yazi` are available immediately in the list and as **Open lazygit** and **Open yazi**
in the command palette and **View → Tasks**. Install these programs first and make them available to
your shell. **Open yazi** opens the current local file’s folder and highlights the file.
With no file to reveal, it opens the project folder or your home folder.
When a launcher supplies `TERM=dumb` and `NO_COLOR`, FiveLeaf enables colors in its local terminals.
Set `env.NO_COLOR` to `"1"` in a task definition to request monochrome output for that task.
Every task opens in an editor tab, including tasks started by a shortcut or **Rerun Last Task**.
The terminal panel has its own shells. You can use or close either surface while the other keeps running.

This workflow takes its task picker and configuration format from [Zed's tasks](https://zed.dev/docs/tasks).

## Configuration

Open **Settings → Terminal → Tasks** to see the effective tasks and where each one comes from.
Choose **Open Global Tasks…** to edit commands shared across projects, or **Open Project Tasks…**
to edit commands for the local folder shown. Both actions are also in **View → Tasks**.
Saving the file and returning to Settings updates the list. **Refresh** also reloads it.

- Global definitions live in `tasks.json` beside FiveLeaf's `settings.json`.
- Project definitions live in `.fiveleaf/tasks.json` under the project folder.
- A project task replaces a global task with the same label; either can replace a built-in task.
- Labels must be unique within each file. Comments and trailing commas are accepted.

Opening the picker reads the files again. A saved edit takes effect on the next launch or rerun.
If a file has an error, the picker shows it with **Edit Tasks** and keeps tasks from valid files and
the built-ins available. Opening a configuration file preserves its existing contents, including invalid JSON.

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

The supported fields are:

| Field | Meaning | Default |
| --- | --- | --- |
| `label` | Name shown in the picker and used by shortcuts | Required |
| `command` | Command text interpreted by your shell | Required |
| `args` | Array of separate arguments appended to the command | `[]` |
| `cwd` | Working directory; relative paths start at the project folder, and `~/` starts at your home folder | Project folder or home |
| `env` | Object of environment variables to add or replace | `{}` |
| `use_new_terminal` | Open a new terminal after an earlier run finishes | `false` |
| `allow_concurrent_runs` | Allow another instance while the same task is running | `false` |

This is a subset of Zed's task format. Other fields are ignored. FiveLeaf reads its own configuration
files; it does not read `.zed/tasks.json` automatically.

## Variables and arguments

`args`, `cwd`, and `env` values accept `$NAME` or `${NAME}`. The following names have both
`FIVELEAF_` and `ZED_` forms, so `$FIVELEAF_FILE` and `$ZED_FILE` mean the same thing:

| Suffix | Value |
| --- | --- |
| `WORKTREE_ROOT` | Open project folder, or home when no project is open |
| `FILE` | Full path of the active file |
| `DIRNAME` | Folder containing the active file |
| `FILENAME` | Name of the active file |
| `RELATIVE_FILE` | Active file's path relative to the project folder |
| `ROW` | Caret row, starting at 1 |
| `COLUMN` | Caret column, starting at 1 |

File variables need an active file. An unavailable file variable or an undefined environment variable
in `args`, `cwd`, or `env` reports an error before starting the task.

Environment values expand against the inherited environment and context variables. They do not
recursively refer to other entries in the task's `env` object. For example, `"PATH": "$PATH:./bin"`
extends the inherited `PATH`. Arguments and the working directory can then use these resolved task
environment values.

Each argument is passed as one literal value after variable expansion. Spaces, quotes, and text such
as `$(...)` inside an argument remain data. Replacement values are expanded once. `command` itself
is shell code and uses that shell's normal syntax; FiveLeaf does not insert context values into its source.
Put file paths in `args` instead of constructing command text from them:

```json
{ "label": "show file", "command": "git diff --", "args": ["$FIVELEAF_FILE"] }
```

The context variables are also available in the shell environment. Shell syntax differs by platform:
POSIX shells use `"$FIVELEAF_FILE"`, while PowerShell uses `$env:FIVELEAF_FILE` in command text.
Windows tasks require PowerShell 7.3 or newer (`pwsh.exe` on `PATH`). FiveLeaf selects its `Standard`
native argument mode to preserve empty arguments and embedded quotes; older versions stop before
running the task. Windows PowerShell (`powershell.exe`) and `cmd.exe` are unsupported. Other supported
shells include sh, bash, zsh, ksh, and fish.

## Reuse and rerun

Running a task again normally focuses its live terminal. After a successful exit, its tab closes;
a failed task keeps its output and **Rerun** action.
**Rerun** and **Rerun Last Task** reload the saved definition and use the file and cursor position from the
original run. If the task is still running, the concurrency rule applies. Switching projects makes
**Rerun Last Task** open the picker for the current project.

Reuse is scoped to the task label and project. The two options combine as follows:

| `allow_concurrent_runs` | `use_new_terminal` | While an instance runs | Once all instances finish |
| --- | --- | --- | --- |
| `false` | `false` | Focus the running instance | Reuse a finished terminal |
| `false` | `true` | Focus the running instance | Open a new terminal |
| `true` | `false` | Open another terminal | Reuse a finished terminal |
| `true` | `true` | Open another terminal | Open a new terminal |

The default `yazi` task opens at the active file on each invocation, even after you navigate elsewhere
inside yazi. A `yazi` task with its own command or arguments uses the configuration you supplied.

Tasks run in local folders. Closing a task terminal or its window ends its process. Task terminals are not restored
or automatically run when FiveLeaf starts, a project opens, or a configuration file changes.

## Keyboard shortcuts

Open **Settings → Terminal → Tasks**, find the task and choose **Set shortcut**. Press the key
combination you want, then `Enter` to save. Select the displayed shortcut to change it or choose
**Remove Keys**. The shortcut follows the task name and runs the current project’s definition.

All task actions are available in **Keyboard Shortcuts**. Commands include `task.spawn`, `task.rerun`,
`task.lazygit`, `task.yazi`, `task.openGlobal`, and `task.openProject`.
To bind a specific task, give `task.spawn` a `task_name` matching its label:

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

Add these entries to `keybindings.json` beside `settings.json`. A named task shortcut launches the
task directly; `task.spawn` without arguments opens the picker.
