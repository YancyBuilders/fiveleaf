# Settings

FiveLeaf's preferences and keyboard shortcuts are two JSON files. You can change them in the
[settings window](#the-settings-window), or edit the files with FiveLeaf or any other editor. Either way, the change
applies in every window right away. You don't need to restart.

## Where the files are

FiveLeaf keeps application settings in its own data folder; existing Mayoi data is left in place:

| System  | Folder                                                       |
| ------- | ------------------------------------------------------------ |
| macOS   | `~/Library/Application Support/FiveLeaf`                        |
| Linux   | `$XDG_CONFIG_HOME/FiveLeaf`, or `~/.config/FiveLeaf` if it's unset |
| Windows | `%AppData%\FiveLeaf`                                            |

| File                | Who writes it | What it holds                                                                     |
| ------------------- | ------------- | --------------------------------------------------------------------------------- |
| `settings.json`     | you and FiveLeaf | [Preferences](#settingsjson): fonts, theme, auto save, vim and more               |
| `keybindings.json`  | you and FiveLeaf | [Keyboard shortcuts](#keybindingsjson), in VS Code's format                       |
| `tasks.json`        | you              | Commands available to all projects, shown in **Terminal → Tasks**                 |
| `state.json`        | FiveLeaf         | Windows, tabs, recent folders and layout                                          |
| `window-state.json` | FiveLeaf         | Each window's position and size                                                   |
| `crash.log`         | FiveLeaf         | FiveLeaf's version and system, then what ended it if it crashed, with every stack    |
| `crashes/`          | FiveLeaf         | The last 10 crash logs, by time; `.sent` or `.unsent` once asked about            |

On macOS and Linux, saves from the settings window or `ff settings` write the user's `settings.json` with mode `0600`.

Plugins, user scripts and what plugins store are in `~/.fiveleaf` instead: see [PLUGINS.md](PLUGINS.md#where-they-are).

To keep the files somewhere else or reuse Mayoi settings, start FiveLeaf with `FIVELEAF_DATA_DIR` set to that folder. `MAYOI_DATA_DIR` is accepted when `FIVELEAF_DATA_DIR` is unset. FiveLeaf then reads and writes all
of these files there instead, which is a way to try a setup without touching your own. Create the folder first, and
quit FiveLeaf before, since a FiveLeaf already running takes over a second one. On macOS:

```sh
mkdir -p ~/fiveleaf-test
FIVELEAF_DATA_DIR=~/fiveleaf-test /Applications/FiveLeaf.app/Contents/MacOS/FiveLeaf
```

## The settings window

**FiveLeaf → Settings…** (⌘,), or **File → Settings…** on Windows and Linux, opens the settings window. It lists every
preference with what it does and its key in `settings.json`. The sidebar has eight groups: **General**, **Appearance**,
**Editor**, **Terminal**, **Version Control**, **Keyboard Shortcuts**, **Agent** and **Plugins**. **Appearance** has
cards for **Theme**, **Window**, **Material** and **Layout**. **Editor** has tabs for **Editing**, **Fonts**,
**Markdown** and **Key Modes** (Vim and Helix). **Terminal** has **General** and **Tasks** tabs.
**Tasks** lists the active definitions, lets you record a shortcut for each task and opens the global or project configuration in the editor.
See [Tasks](TASKS.md#configuration). **Version Control** has **Git** and **Diffs and Merges** tabs.

- **Search** at the top finds preferences in every category, by name, description or key.
- A dot marks a preference that isn't at its default, and the arrow beside it resets it.
- A change applies in every window at once, and is written to `settings.json`.
- **Keyboard Shortcuts** lists every command with its keys. See [Changing shortcuts](#changing-shortcuts).
- With a folder open, the menu at the top right switches between your settings and that folder's own. See
  [Folder settings](#folder-settings).
- `settings.json` and `keybindings.json` at the bottom of the sidebar open the files in the front window.

Some menu items open the window at the preference they're about.

## settings.json

**File → Open Settings (JSON)** opens `settings.json` in a tab. Each key below is optional, and a key you leave out
takes its default.

| Key                     | Default       | Values                                                      | Also set by                                    |
| ----------------------- | ------------- | ----------------------------------------------------------- | ---------------------------------------------- |
| `zoomLevel`             | `0`           | -5 to 8, including decimals                                 | **View → Zoom In / Zoom Out / Reset Zoom**; [shortcuts](#window-zoom) |
| `fontFamily`            | `"monospace"` | Font names, separated by commas                             |                                                |
| `fontSize`              | `13`          | 9 to 28                                                     | **Increase / Decrease / Reset Editor Font Size** in the command palette |
| `fontWeight`            | `400`         | 100 (thin) to 900 (black)                                   |                                                |
| `fontFeatures`          | none          | A list of OpenType features                                 |                                                |
| `lineHeight`            | `1.55`        | 1 to 3, as a multiple of `fontSize`                         |                                                |
| `terminalFontFamily`    | `""`          | Font names, separated by commas                             |                                                |
| `terminalFontSize`      | `0`           | 0, or 9 to 28                                               |                                                |
| `terminalCopyOnSelect`  | `true`        | `true`, `false`                                             |                                                |
| `theme`                 | `""`          | `""` (FiveLeaf), `"matrix"`, `"claude"`, `"qoder"`, `"grokbot"`, `"cursor"`, `"onedarkpro"`, `"ayu"`, `"purplegold"`, `"pumpkin"`, `"witchsbrew"`, `"bloodmoon"`, or a theme's id from `themes`, `~/.fiveleaf/themes` or a plugin | **View → Theme**, **Color Theme…**             |
| `iconTheme`             | `""`          | `""` (FiveLeaf), `"catppuccin-latte"`, `"catppuccin-mocha"`, `"material"`, `"seti"`, `"ayu"`, or a plugin icon theme ID | **Settings → Appearance → Theme → File Icons** |
| `themes`                | `none`        | Themes written here, by id, in the format of a theme file             |                                                |
| `appearance`            | `""`          | `""` (follow the system), `"light"`, `"dark"`               | **View → Theme**                               |
| `windowOpacity`         | `1`           | 0.35 to 1, as the background's opacity                       | **Settings → Appearance → Window → Background Opacity** |
| `micaEnabled`           | `false`       | `true`, `false`                                             | **Settings → Appearance → Material → Mica Material** |
| `micaAnimation`         | `true`        | `true`, `false`                                             | **Settings → Appearance → Material → Animated Mica Background** |
| `micaFrost`             | `0.55`        | 0.15 to 0.95, as the theme's surface tint                    | **Settings → Appearance → Material → Mica Frost** |
| `micaHue`               | `177`         | 0 to 360 degrees                                           | **Settings → Appearance → Material → Mica Hue** |
| `micaIntensity`         | `0.8`         | 0 to 1, as the background light's strength                  | **Settings → Appearance → Material → Mica Intensity** |
| `micaSpeed`             | `1`           | 0.25 to 3, as a speed multiplier                            | **Settings → Appearance → Material → Mica Speed** |
| `sidebarRight`          | `false`       | `true`, `false`                                             | **View → Sidebar on Right**, **Settings → Appearance → Layout → Sidebar on Right**, sidebar button's context menu |
| `hideToolboxButton`     | `false`       | `true`, `false`                                             | **Settings → Appearance → Layout → Hide Toolbox Button** |
| `terminalButtonRight`   | `true`        | `true`, `false`                                             | **Settings → Appearance → Layout → Terminal Button on Right**, terminal button's context menu |
| `panelPosition`         | `""`          | `""` (bottom), `"left"`, `"right"`                          | **View → Panel Position**                      |
| `terminalLocation`      | `""`          | `""` (the panel), `"editor"`                                | **View → New Terminals in Editor Area**        |
| `agentSessionsClaudeDirs` | none        | A list of more Claude Code config folders                   | **Settings → Agent → More Claude Code Folders** |
| `projectTabs`           | `false`       | `true`, `false`                                             | **View → Project Tabs**                        |
| `projectTabStyle`       | none          | A label, color and icon for each folder                     | A square's context menu on the rail            |
| `locale`                | `""`          | `""` (follow the system), `"en"`, `"zh-Hans"`, `"ja"`       | **View → Language**                            |
| `autoSave`              | `true`        | `true`, `false`                                             | **File → Auto Save**                           |
| `quickSuggestions`      | `true`        | Show code suggestions beside the caret while typing | **Settings → Editor → Code Completion** |
| `wordWrap`              | `false`       | `true`, `false`                                             | **View → Word Wrap**                           |
| `lineNumbers`           | `""`          | `""` (absolute), `"relative"`                               | **View → Line Numbers**, **Settings → Editor → Line Numbers** |
| `smoothCaret`           | `true`        | `true`, `false`                                             | **Settings → Editor → Smooth Caret**            |
| `tabSize`               | `4`           | `1` to `8`                                                  |                                                |
| `editorAgentContext`    | `false`       | `true`, `false`                                             | Only this file                                 |
| `previewMermaid`        | `true`        | `true`, `false`                                             | Only this file                                 |
| `previewMath`           | `true`        | `true`, `false`                                             | Only this file                                 |
| `markdownMode`          | `"live"`      | `"live"` (editable preview), `""` (source)                   | **View → Open Markdown As**                    |
| `vim`                   | `false`       | `true`, `false`                                             | **View → Vim Mode**                            |
| `editorMode`            | `""`          | `""` (neither), `"vim"`, `"helix"`                          | **View → Vim Mode**, **View → Helix Keys**     |
| `keymap`                | `""`          | `""` (FiveLeaf), `"vscode"`, `"jetbrains"`, `"sublime"`, or a plugin preset ID | **Settings → Keyboard Shortcuts → Keymap** |
| `vimCtrlKeys`           | `false`       | `true`, `false`                                             | Only this file                                 |
| `blame`                 | `true`        | `true`, `false`                                             | **Git → Inline Blame**                         |
| `diffLayout`            | `""`          | `""` (side by side when wide enough), `"split"`, `"inline"` | A diff's **Layout** button                     |
| `diffCrossFiles`        | `""`          | `""` (say it, and cross on one more step), `"always"`, `"never"` | Only this file                            |
| `diffHighlight`         | `""`          | `""` (changed words), `"line"`, `"char"`                       | A diff's **Highlight** menu                |
| `diffWhitespace`        | `""`          | `""` (exact), `"eol"`, `"all"`, `"blank"`                      | A diff's **Whitespace** menu               |
| `diffContext`           | `3`           | `0` to `100` unchanged lines around each change                | A diff's **Context** menu                  |
| `diffSyncScroll`        | `true`        | `true`, `false`                                                | A diff's **Synchronized Scrolling** menu   |
| `diffAlignedRows`       | `true`        | `true`, `false`                                                | A diff's **Layout** menu                   |
| `mergeAutoApply`        | `true`        | `true`, `false`                                                | **Apply Automatically** in a merge's menu  |
| `explorerCompactFolders` | `false`     | `true`, `false`                                                | **View → Compact Folders**                 |
| `searchTree`            | `false`       | `true`, `false`                                             | **Flat List / Folder Tree** in Search          |
| `scmTree`               | `false`       | `true`, `false`                                             | **Flat List / Folder Tree** in Source Control  |
| `crashReports`          | `""`          | `""` (ask), `"always"`, `"never"`                           | **Don't ask again** when FiveLeaf asks           |
| `plugins`               | none          | Each plugin's own settings, and `"enabled": false` to turn it off | The **Plugins** page of the settings window |
| `agentServers`          | `{}`          | ACP/CLI agents by ID, with commands, environment, tags and timeouts | **Settings → Agent → Registered Agents** |
| `copilot`               | `{}`          | Registered Agent ID and optional model ID for suggestions and commit drafts; empty is off | **Settings → Agent → Copilot** |
| `copilotInlineSuggestions` | `false` | Offer an inline completion after typing pauses | **Settings → Agent → Inline Copilot Suggestions** |
| `aiSecretFiles`         | none          | Files, paths, folders or globs whose contents never go to Copilot | **Settings → Agent → Files Excluded from Copilot** |
| `agentRolePresets`      | `{}`          | Role overrides by stable ID | **Settings → Agent → Role List** |
| `agentRoles`            | `{}`          | Named roles with their Agent, model and prompt | **Settings → Agent → Role List** |
| `formatOnSave`          | none          | A boolean per language, with `"*"` as the default on explicit saves | **Settings → Plugins → Formatting → Format on Save** |
| `formatters`            | none          | The formatter of each language, where plugins offer several | Only this file                                 |

### Language

- **`locale`** is the language FiveLeaf shows: English, Simplified Chinese (`"zh-Hans"`) or Japanese (`"ja"`). Left
  empty, FiveLeaf shows the first of these that the system's preferred languages list, and English otherwise. Another tag
  of the same language works too, as `"zh"` or `"ja-JP"`. The menus, the command palette and every window change at
  once. The command palette also finds commands by their English names. In Japanese, the parts that MyGo draws itself,
  as the editor's own menu and the native dialogs' buttons, stay in English.

### Window zoom

`zoomLevel` scales all window content, including project tabs, the sidebar, icons, buttons, editors and terminals.
The scale is `1.2 ^ zoomLevel`: `0` is 100%, `1` is 120%, and `1.5` is about 131.5%.
Negative levels make the interface smaller. Decimal values such as `0.5` are supported.
Changes apply to every window as you save `settings.json`, and stay after a restart.

| Action | macOS | Windows / Linux |
| ------ | ----- | --------------- |
| Zoom in (level +1) | `⌘=` | `Ctrl+=` |
| Zoom out (level -1) | `⌘-` | `Ctrl+-` |
| Reset (level 0) | `⌘0` | `Ctrl+0` |

These shortcuts zoom the window even when an image has focus.

Image zoom has no default shortcut; use `⌘` (macOS) or `Ctrl` (Windows/Linux) with the scroll wheel, or **Zoom In Image / Zoom Out Image / Reset Image Zoom** in the command palette. Editor font size has its own commands there too: **Increase / Decrease / Reset Editor Font Size**.

To get the old behaviour of `⌘=` / `⌘-` / `⌘0`, bind `editor.fontZoomIn`, `editor.fontZoomOut`, `editor.fontZoomReset`, `image.zoomIn`, `image.zoomOut` or `image.zoomReset` in `keybindings.json`.

### Fonts

- **`fontFamily`** is the code's font. A list falls back in order, as `"Iosevka, monospace"`.
- **`fontWeight`** picks the closest of the weights the font has.
- **`fontFeatures`** turns OpenType features on and off: `["calt=0"]` turns off most coding ligatures, and `["ss01"]`
  turns a stylistic set on.
- **`terminalFontFamily`** and **`terminalFontSize`** are the terminal's. Left empty, the terminal uses `fontFamily`.
  An installed Nerd Font precedes `monospace` for icons that the chosen font lacks.
  and half a point under `fontSize`. Changing the editor font size leaves an explicit terminal size alone;
  window zoom scales both without changing their base sizes. The terminal also takes
  `fontWeight` and `fontFeatures`.

The editor lines up columns, indent guides and the caret in cells as wide as the font's `0`, so a font whose letters
differ in width lines them up only roughly.

### The rest

- **`theme`** selects a color theme. An empty value selects the built-in FiveLeaf theme, which follows `appearance`.
  The Matrix, Claude, Qoder, Grok Bot, Cursor, One Dark Pro, Ayu, Purple Gold and the Halloween themes each have a plugin.
  The Halloween themes keep their original setting values,
  `"pumpkin"` for Jack-o'-Lantern, `"witchsbrew"` for Witch's Brew and `"bloodmoon"` for Blood Moon. One Dark Pro, The Matrix, Purple
  Gold and the Halloween themes are always dark, so they ignore `appearance`. A theme in `~/.fiveleaf/themes` is named by
  its file without `.json` or `.jsonc`, and its file may set `dark` to be always dark. A theme under `themes` is named
  by its key there: see [THEMES.md](THEMES.md).
- **`iconTheme`** selects file and folder icons independently of the color theme. FiveLeaf File Icons,
  Catppuccin Latte and Mocha, Material Icon Theme, Seti and Ayu each have a plugin. Changing the selection updates open windows immediately.
  An unavailable pack uses basic file and folder symbols until the saved choice is available again. See [ICON-THEMES.md](ICON-THEMES.md).
- **`windowOpacity`** sets the background's opacity from 0.35 to 1. At 1 the background is solid; lower values reveal
  the desktop through frosted glass. Text and controls keep their opacity. It applies to every main window and the
  settings window; systems without a native background material keep an opaque background.
- **`micaEnabled`** adds a frosted material over the selected color theme, in light or dark appearance. It also
  works without a native window material. An older `"theme": "mica"` choice becomes this switch with FiveLeaf's
  default color theme.
- **`micaAnimation`** controls background flow. Setting it to `false` keeps a static frosted background; Reduce
  Motion also pauses it. Visible windows keep flowing while the settings window has focus.
- **`micaFrost`**, **`micaHue`**, **`micaIntensity`** and **`micaSpeed`** adjust the surface tint, light color,
  strength and flow speed. More frost shows more of the selected theme's surface color. The settings window
  previews slider changes while dragging; releasing saves the value and applies it to all windows.
- **Settings → Appearance → Material → Import… / Export…** saves and restores a portable JSON configuration, including all six
  Mica settings and background opacity. Import keeps the selected color theme and other preferences. These
  actions also work while Mica is off. Invalid, incomplete or unsupported files leave settings unchanged.
- **`smoothCaret`** moves the caret smoothly with a connected trail that catches up after it stops. Mouse clicks,
  keyboard navigation and selection moves glide between nearby and distant places, including Vim and Helix counts,
  word, line and document jumps, in source and Live Preview. Dragging updates the selection immediately while the
  caret and trail keep following its endpoint. Setting it to `false` disables both; reduced motion, double clicks,
  scrolling, zooming and loss of focus also place the caret immediately without a trail.
- **`sidebarRight`** puts the sidebar and its bottom button on the right when `true`, or the left when `false` (the
  default). Right-click the sidebar button and choose **Bottom Left** or **Bottom Right** to change it. **View →
  Sidebar on Right** and **Settings → Appearance → Layout → Sidebar on Right** change the same setting; it applies to every
  window and is saved between launches.
- **`hideToolboxButton`** takes the Toolbox button out of the sidebar when `true`. The tools are still in the
  command palette, and `$` lists only them.
- **`terminalButtonRight`** puts the terminal button at the bottom right when `true` (the default), or the bottom
  left when `false`. Right-click the terminal button and choose **Bottom Left** or **Bottom Right**, or use
  **Settings → Appearance → Layout → Terminal Button on Right**. Its choice is independent of the sidebar and terminal panel
  positions, applies to every window and is saved between launches.
- **`lineNumbers`** chooses absolute (`""`, or `"absolute"`) or relative (`"relative"`) editor line numbers. Relative
  numbers show the distance from the caret's line; that line keeps its absolute number. The choice applies to every
  window and is saved between launches.
- **`panelPosition`** is where the terminal panel is: `""` or `"bottom"` below the editors, `"left"` or `"right"` at a
  side of the window. At a side it sits past the sidebar, so with the sidebar on the same side the panel is the outer
  of the two.
- **`terminalLocation`** chooses where **New Terminal** and the Explorer's **Open in Terminal** put the first terminal
  in a window: `""` in the panel, or `"editor"` in an editor tab. Later terminals join the existing ones. **Toggle
  Terminal** (⌃`) brings the same shell sessions back to the panel. Tasks always open in their own editor tabs.
- **`agentSessionsClaudeDirs`** lists more Claude Code config folders whose sessions the **Sessions** view shows, as
  `["~/work/claude"]`. Without it the view already reads `~/.claude`, each `~/.claude-*` folder and the folder
  `CLAUDE_CONFIG_DIR` names. It stays on each machine.
- **`projectTabs`** keeps every folder open in one window, as project tabs on a rail on its left, rather than a
  window for each. Changing it gathers the windows into one, or parts them, at once.
- **`projectTabStyle`** gives a project's square on the rail its own label, up to two characters, in place of its
  initials or logo, and its own color, one of `"red"`, `"green"`, `"yellow"`, `"blue"`, `"magenta"` and `"cyan"`, by
  the project's folder: `{"/Users/me/Downloads": {"label": "DL", "color": "cyan"}}`. A remote folder's key is
  `/ssh:host/path`. `"icon"` names a picture in the `project-icons` folder beside `settings.json`, which **Choose
  Icon…** and dropping a picture on the square copy there, shown in place of the project's logo. The folders and the
  pictures are this machine's.
- **`terminalCopyOnSelect`** copies what you select in a terminal to the clipboard as you let go of the pointer,
  and what Select All selects. Changing it applies to the terminals already open.
- **`autoSave`** saves a changed file a moment after you stop typing, and when you leave it.
- **`wordWrap`** wraps long lines at the edge of the editor. **⌥Z** wraps or unwraps one tab only, until this
  setting changes.
- **`tabSize`** is how many columns a tab takes in the editor, the same on every system. It doesn't change
  what a key inserts: a file indented with spaces keeps its own indent size.
- **`editorAgentContext`** shows a **+** beside the line under the pointer in ordinary files. Click it
  to comment on that line or the selected lines in an inline review card; an empty comment collects the
  code alone. Collect snippets across files and send them together from the editor toolbar. It is off
  by default; changes apply immediately and do not discard pending items. Closing the draft’s tab,
  starting another comment, or jumping to code keeps its text in the Review queue. **Send** saves the
  current draft before sending the batch.
- **`previewMermaid`** draws the `mermaid` code blocks of Markdown as diagrams in Live Preview. Turn it off to
  show them as code. A `.mmd` or `.mermaid` file's own preview draws its diagram either way.
- **`previewMath`** draws the LaTeX math of Markdown, between `$` and `$`, `$$` and `$$`, or in a `math` code
  block, as formulas in Live Preview, but for `math` code blocks, which show as code. Turn it off to show the
  source.
- **`markdownMode`** defaults to editable Live Preview for Markdown files, including `.mdx`. Set it to `""` to
  open files as source. **Source** beside the file path switches the current tab to raw Markdown, and **Preview**
  switches it back. `⌥⌘V` does the same. The session remembers each tab’s view.
- **`diffHighlight`** chooses whether changed lines show word-level, character-level or line-level tinting. The
  highlighting changes how a diff looks; block actions still use whole line hunks.
- **`diffContext`** keeps that many unchanged lines around each diff hunk visible and folds the rest.
- **`diffAlignedRows`** keeps the rows of a split diff's sides aligned: a line the one side lacks holds a space on the
  other. Turn it off, and each side's lines close up where the other has lines it lacks, the sides reading in streams
  of their own, which scroll on their own whatever `diffSyncScroll` has.
- **`diffSyncScroll`** keeps the two sides of a split diff aligned while scrolling. Turn it off to scroll each side
  independently.
- **`vim`** starts editors in vim's normal mode. It is the older way to choose vim; `editorMode` is the newer one, and
  takes precedence when set.
- **`editorMode`** starts editors in vim's or helix's normal mode, or in neither with `""`. Vim and Helix are
  exclusive. In either mode, Space shows the [Leader menu](FEATURES.md#leader-menu) in the bottom-right corner while
  the editor is in normal mode; Vim visual mode supports it too. `Ctrl-w` opens the window menu for IDE columns.
  Insert mode keeps Space for typing. Bind `go.leaderMenu` in `keybindings.json` to open the Leader menu with another shortcut.
- **`vimCtrlKeys`** gives ⌃F, ⌃B, ⌃R, ⌃A and ⌃X to vim's normal mode on Windows and Linux, rather than to Find,
  Toggle Sidebar, Open Recent, Select All and Cut. On macOS, where those commands use ⌘, vim always has them.
- **`blame`** shows who last changed the caret's line, and when, at the end of the line.
- **`explorerCompactFolders`** shows each chain of folders that contain only one child folder on one Explorer row,
  as `src/main/java/com/example`. The row's actions use the last folder in the chain. Turn it off to show the
  folders separately, including when you want to rename or move an intermediate folder. The Explorer folder
  header's context menu also has **Compact Folders**. Dependency and build folders such as `node_modules`,
  `.venv` and `target` stay separate and are read only when expanded.
- **`scmTree`** lists Source Control's changes in a tree of their folders, rather than by path.
- **`searchTree`** groups search results by folder, independently of Source Control's layout.
- **`crashReports`** is what FiveLeaf does at launch with the crash logs of the last runs: `""` asks whether to send them
  to Mon, `"always"` sends them and `"never"` doesn't. A report is anonymous and has your home folder taken out, but not
  other paths, such as `/Volumes/...` or `/opt/...`. **Show Report** in the question opens the logs as they would be
  sent. Reports are sent only when `FIVELEAF_MON_URL` explicitly configures a service. Enter answers **Don't Send**.

### Plugins

- **`plugins`** holds each plugin's settings under its name, as its `plugin.json` declares them, and `"enabled":
  false` turns a plugin off. A folder can't set it, so that a repository can't turn on a plugin. See
  [PLUGINS.md](PLUGINS.md).
- **`formatOnSave`** formats the document on explicit saves. The switch sets the default for all supported languages.
  Use `{"go": true}` for Go alone, or `{"*": true, "json": false}` for every supported language except JSON.
  Automatic saves write the current text. A formatter plugin takes priority, followed by the language server and
  then the built-in Go and JSON formatters. A save can use the built-in formatter while its language server starts.
- **`formatters`** chooses the formatter of a language where plugins offer several, by the formatter's id or its
  plugin's name, as `{"typescript": "prettier"}`.

```json
{
  "plugins": { "word-count": { "wordsPerMinute": 200 }, "prettier": { "enabled": false } },
  "formatOnSave": { "typescript": true },
  "formatters": { "typescript": "biome" }
}
```

### ACP agents

Agent registrations and roles have their own **Settings → Agent** page.
**Registered Agents** lists custom registrations and agents from enabled plugins. A local definition with
the same ID appears once, marked **Plugin override**. The link on a plugin-owned row opens its plugin.
Use **Add Custom Agent…** to register your own ACP or CLI program
with separate name, executable, argument and environment fields. The same form edits
existing definitions; removing one offers **Undo**. **Configure Agents…** in the Agent
header or command palette opens this section. **Edit in JSON** remains available.

`agentServers` adds external agents to the **Agent** view. Each map key is a stable ID; `name` is optional
and defaults to that ID. `command` is one executable, `args` is its argument list, and `env` adds environment
variables to those inherited by FiveLeaf. ACP programs start when you connect; CLI programs start when sent
a message. Commands run without a shell; put flags in `args` and use an absolute executable path if it is not on `PATH`.

- `protocol` defaults to `"acp"` for protocol connections; choose `"cli"` for one-shot commands.
- `workingDirectory` defaults to the project folder. Relative paths resolve from it. ACP session and editor
  paths still use the project.
- `capabilities` defaults to `[]`. Routing tags such as `"code"`, `"reasoning"` and `"search"` grant no permissions.
- `disabled` defaults to `false`. Set it to `true` to pause this agent and disconnect its active work.
- `timeoutSeconds` defaults to `120` and accepts 1–3600 seconds; `0` uses the default.
- `promptArgument` defaults to `false`. For CLI, `true` appends the prompt as one final argument instead of stdin.

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

Install OpenCode before using the first example. The Gemini example needs Node.js and downloads the pinned
package on its first connection. Sign in with the corresponding agent's CLI first; the agent owns its
credentials and model configuration. The CLI example needs an installed, authenticated Claude CLI.
Agent conversations currently use local project folders.

**Import… / Export…** exchanges a JSON file with `version: 1` and an `agentServers` map, or `version: 2`
when custom roles are included in an `agentRoles` map. Files with customized built-ins use `version: 3`
and include `agentRolePresets`. An export with a named role that follows the current Agent uses `version: 4`.
Untouched built-ins and plugin-owned agents are not exported.
Export includes custom definitions and plugin overrides, including environment values, and writes a private file.
Import merges new agent IDs and role names and accepts equivalent definitions; a conflict or invalid entry rejects
the whole import. All maps are applied together. Registration and import do not start any program.
Use **Settings → Agent → Conversation settings** to edit the last focused chat's system prompt and
context window. Roundtables use the moderator’s assigned Agent and model to coordinate discussions.

This is a user setting: a repository's `.fiveleaf/settings.json` cannot add or replace agent programs.
Alternatively, install and enable an Agent by name in **Settings → Plugins**. See
[ACP agent plugins](PLUGINS.md#acp-agent-servers) for their requirements and custom plugin declarations.
See [Agent chats and roundtables](AGENTS-ACP.md) for routing, context and code review.

### Copilot

In **Settings → Agent → Copilot**, choose **Configure Copilot…**, select a registered Agent and save.
Leave the model blank for the Agent’s default, use **Load models** to choose from an ACP Agent’s catalog,
or enter an exact model ID. To select a model for a CLI Agent, include `{model}` in its registered arguments.
Switching Agents clears the model field. **Off** disables Copilot. The choice is saved in user settings,
independently of models selected for chat or roles.

```json
{ "copilot": { "agent": "my-agent", "model": "my-model" } }
```

In a local project, use **Selection → Suggest Text with Copilot** to request a completion at the caret.
Choose the suggestion to insert it or replace the selection.

Enable **Settings → Agent → Inline Copilot Suggestions** to send nearby code or prose to that Agent
after a typing pause. The suggestion appears at the caret. Press `Tab` to insert it or `Esc` to
dismiss it, or click the matching control in the editor. Undo removes an accepted suggestion in one step.
When the word completion menu is open, its candidates take keyboard priority. Files matching
`aiSecretFiles` are excluded from automatic suggestions.

**Git → Draft Commit Message with Copilot** uses the same Agent and model to fill the commit message box.
You can edit the draft before committing.
The draft uses staged changes, or working-tree changes when nothing is staged. Use `aiSecretFiles` in
user settings to omit the contents of additional files from commit drafts, for example `["private-notes.txt"]`.

### Agent roles

The invitation list for a new roundtable offers eleven [built-in roles](AGENTS-ACP.md#configure-and-use-roles).
In **Settings → Agent → Role List**, each row previews its effective prompt. Edit the row to read the
full prompt and choose an Agent and model. **Add Role…** opens a blank role. **Copy Role** opens a
draft with a new name and the source role’s current Agent, model and full prompt. Save to create the role.
**Current agent** keeps the role unbound; an empty model uses the Agent's default. Use **Load models**
to query the selected ACP adapter if no open chat has its list, or enter a model ID yourself. The lookup
does not send a prompt. New invitations use the Agent, model and prompt saved for the role.

**Restore defaults** in a preset row or its editor resets that preset to the current Agent, the Agent’s
default model and its localized prompt. **Restore all preset role defaults** resets every preset.
Either action can be undone from the notification.

Only edits are saved in `agentRolePresets`, under `explore`, `plan`, `implement`, `test`, `review`,
`diagnose`, `document`, `requirements`, `interaction`, `compatibility` or `moderate`.
For example, `"agentRolePresets": {"review": {"agent": "opencode"}}`
assigns Code Reviewer without replacing its localized prompt. Omit `prompt` for the built-in text or set
it to `""` for no role prompt. A custom role of the same name is independent.

Saved and imported named roles appear in the same list. Their names are the keys in `agentRoles`.
An entry with `"followCurrentAgent": true` leaves `agent` and `model` empty and resolves its Agent
when used. An entry with an assigned Agent can also specify a model.

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

Choose a role in **Roundtable → Invite members**. Each member uses its own saved prompt and model,
including when several roles share an Agent.
Leave `model` empty to use the Agent's default, or enter an exact model ID supported
by that Agent. An ACP adapter must advertise model selection for an explicit model. A CLI Agent must have a
literal `{model}` placeholder in its argument array, such as `args: ["--print", "--model", "{model}"]`.
FiveLeaf replaces that placeholder with one argument value and does not invoke a shell. Unsupported models
and missing placeholders prevent sending and keep the draft. A CLI template containing `{model}` requires
an explicit role model; use an Agent definition without the placeholder to keep the CLI's default arguments.

In an idle direct ACP chat, use the composer's **Model** menu to choose its model.
The context indicator below the composer displays the adapter's reported
token occupancy and capacity; adapters that have not reported usage show **Context —**.

Role names support Chinese and spaces within the name. Up to 128 roles are supported, with a 64 KiB prompt
limit per role. A role with a missing or disabled Agent remains saved and visible but cannot send.
Renaming creates a new target; the old conversation keeps its history but requires choosing an available target.
`agentRoles` is a user setting and cannot be overridden by a repository's `.fiveleaf/settings.json`.

### Share roles and create them from an Agent

In **Settings → Agent → Role List**, **Export Roles…** saves `fiveleaf-roles.json`.
It contains your named roles and every built-in preset. Each preset has a stable ID, its localized
`name` and `defaultPrompt`, and any `agent`, `model` or `prompt` override you saved. The localized
fields are for reference. Set `prompt` to customize the preset. An explicit `""` keeps it empty.
**Import Roles…** also reads the role definitions from version 1–4 Agent configuration files.
Agent registrations and their environment values stay in the separate Agent configuration export.

An Agent with shell access can create several named roles with one command. The role names are the
keys of `roles`. Set `followCurrentAgent` to use the available Agent or supply an `agent` ID and
an optional `model` to use a specific one.

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

`ff roles list` prints the same bundle as **Export Roles…**. A successful import prints the number
of named roles created and preset overrides applied, for example `{"created":2,"presetsApplied":0}`.
Repeating it returns zero for both. A conflicting definition or invalid entry rejects the whole batch.
The limit is 1 MiB per file or request payload and 128 saved named roles per profile.

To call the local JSON API directly, connect to the Unix socket printed by `ff roles socket` while
FiveLeaf is running. Send one newline-terminated JSON object per connection and read the response
before closing the socket. The socket is private to the current user. `list` and `export` return the
bundle under `data`. `import` accepts the same bundle and returns counts under `data`. Failures
return an `error` string. For example, the following request creates a named role.

```json
{"roles":{"action":"import","bundle":{"version":1,"roles":{"Release reviewer":{"followCurrentAgent":true,"prompt":"Verify the release notes."}}}}}
```

### Example

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

### Good to know

- **It's plain JSON.** Comments and trailing commas don't parse, though `keybindings.json` allows comments.
- **FiveLeaf rewrites the file** in its own formatting soon after it starts, when you open it with ⌘, and when you
  change a preference from a menu, a button or zoom. It writes every key, defaults included, and drops any key it
  doesn't know.
- **FiveLeaf ignores a file that doesn't parse,** and says nothing. The preferences stay as they were until you fix it
  and save again.
- **FiveLeaf quietly corrects values out of range.** A `fontSize` of 40 becomes 28, and an empty `fontFamily` becomes
  `"monospace"`.
- **Older files still work.** Themes used to be appearances, so FiveLeaf reads `"appearance": "matrix"` or `"claude"`
  as that `theme`.

### From the command line

Use the [`ff` command](FEATURES.md#the-ff-command) to read or change preferences even when FiveLeaf is closed.

```sh
ff settings                         # every preference, its key and its value
ff settings get theme fontSize      # a value a line
ff settings set theme=claude fontSize=16
ff settings unset theme             # its value of a first launch again
ff settings set fontFeatures+=ss01  # one item more of a list
ff settings set projectTabStyle.'<folder>'.label=MG
ff settings --folder set vim=true   # the current folder's own file
```

- **A value is JSON when it is JSON,** so `16`, `true` and `["calt=0"]` give a number, a switch and a list, and any
  other text is the text itself, as `theme=claude` and `fontFamily=Fira Code`, `""` being the empty one.
- **A list changes an item at a time with `+=` and `-=`,** so a long list needs no writing out: `fontFeatures+=calt=0`
  appends one that isn't there already, and `fontFeatures-=calt=0` takes one away, the last leaving no list.
- **A project's look on the rail is told a field at a time,** as `projectTabStyle.'<folder>'.label=MG`, or `.<folder>`
  for the whole of it, `{"label": "MG", "color": "cyan"}`. A field is `label` of up to two characters as you see
  them, or `color` of `red`, `green`, `yellow`, `blue`, `magenta` or `cyan`; a field left empty takes it away, and a
  look left with no field is the project's own. A folder with dots of its own needs no quotes, as
  `projectTabStyle.~/src/my.app.label`, and one told from the current folder or from `~` is the folder whole. An icon is no field here: it names a
  picture the app copied into its own folder, which the rail's menu chooses, so a look the command changes keeps the
  icon it has.
- **A value FiveLeaf has none of changes nothing,** and says what the preference takes instead: an unknown key, a value
  of the wrong type, a number out of its range or a choice that isn't one of its options exits with status 1.
- **Only the preference you change is written,** the others stay as the file has them, a key a line in the order of
  their names, and the folder is made when the file is new.
- **A FiveLeaf running reads the file again** and applies the change at once, as it does one you save in an editor, so
  the window and every other window change with it.
- **`--folder` (`-f`) acts on the current folder's own `.fiveleaf/settings.json`,** of the preferences a folder can
  choose, which `ff settings --folder` lists, and `get` prints `null` of one it doesn't choose. In a subfolder of a
  project it acts on the nearest `.fiveleaf/settings.json` up from it, the one the project reads.

### Folder settings

When `.fiveleaf/settings.json` is absent, an existing `.mayoi/settings.json` is used in place.
The legacy `markdownPreview` toggle still sets the Markdown view when `markdownMode` is absent.
`ff settings --folder unset markdownMode` clears both keys.

A folder can choose some preferences for itself in `.fiveleaf/settings.json`, which overrides yours in that folder's
window. **File → Open Folder Settings (JSON)** opens the file, and creates it if there is none. In the settings
window, choose the folder in the menu at the top right: it lists only the preferences a folder can set, and the arrow
beside one returns it to your setting.

A folder can set these: `autoSave`, `quickSuggestions`, `wordWrap`, `markdownMode`, `previewMermaid`, `previewMath`,
`terminalCopyOnSelect`, `diffHighlight`, `diffWhitespace`, `diffContext`, `diffCrossFiles`, `mergeAutoApply`,
`blame`, `vim`, `editorMode`, `vimCtrlKeys`, `formatOnSave` and `formatters`.

```json
{
  "vim": true,
  "wordWrap": true
}
```

- **FiveLeaf ignores the folder's other keys** and leaves them in the file as they are.
- **A change to one of these preferences goes to the folder's file** while the folder sets it. For example, with
  `vim` in the file, **View → Vim Mode** changes the folder's `vim`, not yours.
- **Saving the file applies it** to the folder's window at once, as does a checkout that changes it.
- **Remote folders have no settings of their own.** Folder settings are part of the folder, which you can commit
  along with it.

## Files FiveLeaf keeps itself

FiveLeaf writes `state.json` and `window-state.json` as it runs. To start fresh, quit FiveLeaf and delete the file.

- **`state.json`** holds the open windows with their folders, columns and tabs, each tab with its caret, selection,
  scroll position and own word wrap. It also holds the folders open in the Explorer, the recent folders and the sessions
  FiveLeaf restores when one opens again, and each window's layout: its sidebar's view, whether it is open and its width,
  and its panel's height and, at a side, its width. Deleting it forgets all of that.
- **`window-state.json`** holds each window's position and size.

Versions before 0.5.1 kept the windows and layout in `settings.json`. FiveLeaf moves them to `state.json` the first time
it starts.

## keybindings.json

**File → Open Keyboard Shortcuts (JSON)** opens `keybindings.json`, beside `settings.json`, in VS Code's format,
comments and all, so a VS Code or Cursor file can be pasted in. Saving it applies it in every window at once. A file that does not
parse is left alone until it does, and a key or a condition FiveLeaf can't read is named in a toast.

- **`key`** is a key, as `"cmd+1"`, or a chord, as `"ctrl+. g c"`. Its modifiers are `cmd`, `ctrl`, `alt` and `shift`.
  While a chord is waiting for its next key, the status bar shows its keys. A key it has no binding of does nothing and
  ends it, and Escape ends it too.
- **`command`** is a command of FiveLeaf's, by its id as `view.sidebar`, or VS Code's name for it, as
  `workbench.action.toggleSidebarVisibility`. A command named with a `-` first takes the key away from that command.
  `runCommands` runs its `commands` in order, each once the edits and the Git commands of the one before are done, and
  stops at an error. `vscode.open` opens the file of its `args`: an absolute path, one from `~`, or one in the folder.
  `type` types the `text` of its `args` in the editor or the terminal, and `workbench.action.terminal.sendSequence`
  sends it to the terminal. Pressing the key of a command FiveLeaf doesn't have says so.
- **`when`** is when it applies: `editorTextFocus`, `editorFocus`, `inputFocus`, `textInputFocus`,
  `filesExplorerFocus`, `terminalFocus`, `terminalHasBeenCreated`, `terminalIsOpen`, `sideBarVisible`,
  `editorHasSelection`, `isMac`, `isLinux` and `isWindows`, with `!`, `&&`, `||` and parentheses. Other names don't hold.

The keys of the file take the place of FiveLeaf's own: the menus drop a key the file binds, ⌘1 to ⌘9 and ⌃R let the file
have theirs, and the command palette shows the file's keys. The shortcuts apply in the editor, the terminal, the
Explorer, diffs, pictures, and the text fields of find, search, quick open and the commit message.

```json
[
  { "key": "ctrl+. s", "command": "editor.action.indentUsingSpaces" },
  { "key": "ctrl+. g p", "command": "git.push" },
  { "key": "d", "command": "deleteFile", "when": "filesExplorerFocus && !inputFocus" },
  { "key": "cmd+1", "command": "workbench.action.toggleSidebarVisibility" }
]
```

### Changing shortcuts

**File → Keyboard Shortcuts…** opens the settings window at its list of commands. Each command shows its keys and
where they come from: **Default** for FiveLeaf's own, **Preset** for the selected keymap, **User** for `keybindings.json`'s, and **Removed** when the file
takes them away. **+1 more** means the file has further keys for the command, with a `when` or `args`.

- **Keymap** lists FiveLeaf's defaults and the installed presets. Install **VS Code Keymap**, **JetBrains Keymap**
  or **Sublime Text Keymap** in **Settings → Plugins** to add that choice or restore a saved selection.
  Keys follow macOS, Windows or Linux. This setting is independent of Vim and Helix editing modes.
  Plugins can add more choices with [`contributes.keymaps`](PLUGINS.md); a missing or disabled preset falls back
  to FiveLeaf and shows a notice, while keeping its ID so it returns when the plugin is available again.
- The selected preset supplies inherited shortcuts. Your `keybindings.json` entries take priority and stay in
  place when you switch presets. For example, `"keymap": "jetbrains"` in `settings.json` selects JetBrains.
- **Search** finds commands by name, id or keys. The keyboard button next to it finds them by keys you press.
- **Click a command**, or its pencil, and press its new keys: two for a chord. Enter saves, Escape cancels and
  Backspace starts over. If other commands have those keys, the dialog says which.
- **The minus** removes the command's inherited keys, and **the arrow** restores the selected keymap's keys.

FiveLeaf writes each change to `keybindings.json` and applies it at once. It changes only the command's entries without
a `when` or `args`, and keeps the file's comments and order. New keys go at the end, with entries that take away
the inherited shortcuts, as VS Code writes them:

```json
{ "key": "cmd+s", "command": "-file.save" },
{ "key": "ctrl+k s", "command": "file.save" }
```

### Windows and Linux shortcuts

The shortcuts are macOS's with Ctrl for ⌘, and the hints show them so, but for those VS Code has others for:

| Command                                  | macOS    | Windows and Linux |
| ---------------------------------------- | -------- | ----------------- |
| Find Next, Find Previous                 | ⌘G, ⇧⌘G  | F3, Shift+F3      |
| Replace                                  | ⌥⌘F      | Ctrl+H            |
| Show the tab 1 to 8, the last            | ⌘1 to ⌘9 | Alt+1 to Alt+9    |
| Save All, Open Folder in New Window      | ⌥⌘S, ⌥⌘O | none              |
| Focus Next Column, Focus Previous Column | ⌥⌘→, ⌥⌘← | none              |

Ctrl with Alt is AltGr there, which types text, so no shortcut takes it: bind those commands in `keybindings.json`,
as `{ "key": "ctrl+k s", "command": "workbench.action.files.saveAll" }`. A file can tell the systems apart with
`isWindows`, `isLinux` and `isMac`.
### `diffWhitespace`

Controls which whitespace participates in diff comparison: `""` for exact,
`"eol"` to ignore line-end spaces and tabs, `"all"` to ignore whitespace,
or `"blank"` to also treat whitespace-only lines as blank. The filter changes
the diff display and navigation while file text and block actions keep using
the original lines.
