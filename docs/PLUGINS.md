# Plugins

Plugins add commands, react to the editor, check and format code, draw views and previews, and bring keymaps, color and icon themes,
languages, snippets, language servers and ACP agents. Their JavaScript or TypeScript runs in FiveLeaf's own engine,
[moejs](https://github.com/MoeForge/moejs): no Node, no build step, and no webview unless a plugin asks for one.
A plugin that only declares keymaps, themes or other static contributions needs no code.

The API is new and may still change before 1.0. A plugin says which versions of FiveLeaf it works with in `fiveleaf`.
Existing Mayoi plugins can still import `mayoi`, call `settings.mayoi()`, and use the `mayoi` version field.
FiveLeaf checks that field against Mayoi 0.20.1, the version FiveLeaf is based on.
When both version fields are present, `fiveleaf` takes precedence. New plugins use `fiveleaf` and `fiveleaf.d.ts`.
The marketplace and `ff plugin search` read the [official FiveLeaf index](https://raw.githubusercontent.com/YancyBuilders/fiveleaf-plugins/main/index.json).
If the server returns an error, **Settings → Plugins** keeps the last list shown in that window or uses the list shipped with FiveLeaf.
Choose **Try Again** to refresh it. `ff plugin search` also uses the shipped list during an outage. Installing a listed plugin still requires access to its Git repository.

## Using plugins

### Installing

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

Use `repository#path/to/plugin` when `plugin.json` lives in a subdirectory. A plain Git URL selects the repository
root. Append `@ref` after the path to follow a branch or pin a tag or commit. A local Git path such as
`./fiveleaf-plugins#material-icons` works the same way. Each plugin gets its own folder and source record.
An install by name takes the newest release in the index that this FiveLeaf can run. **Update** and `ff plugin outdated`
use that compatibility check for plugins that follow the index. An update can follow a release to a new subdirectory
in the same repository. A version typed after `@`, as `one-dark-pro@1.0.0`, stays at that version. Older installed
release records without a saved update choice stay at their installed versions. Run `ff plugin install <name>` to
switch a fixed release to following the index. If the source moved while the plugin was fixed, remove the old
installation before installing by name.

Direct Git installs follow their original repository and ref. A branch, tag or commit selected by name, such as
`one-dark-pro@main`, also keeps the plugin's subdirectory from the catalog. Git updates work independently of the index.

Open **Settings → Plugins** to search installed plugins and the index together. Combine a category with
**Installed** or **Not Installed**. Disabled plugins also appear under **Installed**.
Install from a result or use **Plugin Actions → Install from Git…**. Open an installed plugin to manage its settings.

Choose **Install Dev Plugin…** and select the folder containing `plugin.json` to load a local plugin.
FiveLeaf follows edits in that folder. Removing the plugin unlinks it and keeps its source files.

The `ff plugin` command works on the plugins folder, so it works with FiveLeaf closed; a FiveLeaf that runs loads what
changed at once.

Plugins are trusted as settings are: installing one means trusting its code, which runs with your rights. Install
plugins you know.

### Where they are

`FIVELEAF_DATA_DIR` takes precedence over `MAYOI_DATA_DIR`. Without either variable, FiveLeaf uses
`~/.fiveleaf`, or an existing `~/.mayoi` when `~/.fiveleaf` does not exist. The paths below show the new default.

| Folder                              | What                                                        |
| ----------------------------------- | ----------------------------------------------------------- |
| `~/.fiveleaf/plugins/<name>/`          | Installed plugins, a folder each                            |
| `~/.fiveleaf/plugins/plugins.json`     | Each Git installation's source, version, commit and update choice |
| `~/.fiveleaf/scripts/`                 | User scripts                                                |
| `~/.fiveleaf/themes/`                  | Themes of the user's own; see [THEMES.md](THEMES.md)        |
| `~/.fiveleaf/storage/<name>/`          | What plugins store                                          |

On Windows, `~` is `%USERPROFILE%`. With `FIVELEAF_DATA_DIR` set, these folders are in that folder instead.

Install **Word Count** for reading time in the status bar or **TODOs** to find TODO, FIXME and HACK comments
in the sidebar. These tools, language servers, formatters and Agent plugins start off. After installing a plugin,
enable it on the **Plugins** page or set `"enabled": true` in its settings. See [ACP agents](#acp-agent-servers).

### Toolbox

Enable a tool under **Settings → Plugins → Tools**. Open the **Toolbox** button beside **Sessions**, or type `$`
in the command palette. Search by name or feature and press `Enter` to open the tool in an editor tab. Selecting it
again brings its tab forward with your input intact. Use **Switch Tool** or `Cmd+K` (`Ctrl+K` on Windows and Linux)
in an official tool to open the palette. Close the tool with its tab's close button.

- **Timestamp** includes timestamp conversion, time zones and a date calculator.
- **Base64** encodes UTF-8 text and decodes Base64 back to text.
- **JSON** includes formatting, format conversion and type generation.
- **URL** encodes and decodes URLs and query components.

The four official plugins use the original DevTool Vue components packaged with FiveLeaf, including their feature tabs,
styles, icons and editors. You can install and enable them individually, for example `ff plugin install toolbox-json`.

#### Vue and React tools

A tool tab renders the plugin's built page with its own DOM, stylesheets, scripts and assets. To start a Vue project,
run the following commands. For React, use `--tool=react` on the first line. Plain HTML tools use `--tool`.

```sh
ff plugin new my-tool --tool=vue
cd my-tool
npm install
npm run build
ff plugin dev .
```

Edit `App.vue` or `main.jsx`. Run `npm run build -- --watch` to rebuild as you edit. Ship `plugin.json` and the complete
`dist/` directory, including styles, fonts, images and lazy-loaded chunks. Users can run the published tool offline.
FiveLeaf serves the directory containing the entry HTML as the root of the tool's origin. The Vite templates set
the build base to `./`. For internal page navigation, a hash router works without a server-side route fallback.

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

Use `window.mygo.call("Toolbox.Copy", text)` to copy from a page. The other Toolbox methods use the same calling convention.

| Call | What it does |
| --- | --- |
| `Toolbox.Copy(text)` | copies text with the editor's clipboard |
| `Toolbox.Theme()` | returns `{ dark, colors }`, the palette's colors |
| `Toolbox.SetDetail(false)` | returns to the tool picker |
| `Toolbox.ManagePlugins()` | opens **Settings → Plugins → Tools** |
| `Toolbox.OpenTools()` | opens the main window's tool palette |
| `Toolbox.Close()` | closes this tool's editor tab |

`window.mygo.on("toolbox-theme", handler)` receives later theme changes. The page chooses how to apply them.

### When something goes wrong

- **Plugins → Show Plugin Log** opens a tab of what plugins logged and the errors they hit, the newest at the bottom.
  Filter it by plugin, by level and by text; it follows new lines while scrolled to the end. **Copy** copies the lines
  shown, and **Clear** empties it. It keeps the last 10,000 lines.
- **Plugins → Show Running Plugins** opens a table of the plugins: state, version, the time their JavaScript ran, the
  memory they keep, when they started, how often they crashed, and their last error. Click a column's head to sort by
  it. It refreshes every second while it shows. Each row can restart its plugin, turn it off, open its details, or
  show its log.
- A command's JavaScript that runs for 10 seconds without a break is stopped, and so is a plugin that keeps more than
  256 MiB. A plugin that stops 3 times in a row is off until **Plugins → Reload Plugins**.
- `ff --disable-plugins` starts FiveLeaf with installed plugins disabled.

### Settings of plugins

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

See [SETTINGS.md](SETTINGS.md#plugins).

## Writing a plugin

```sh
ff plugin new word-stats   # writes ./word-stats
cd word-stats
ff plugin dev              # loads it, and again on each save
```

`ff plugin new` writes `plugin.json`, `main.ts`, `fiveleaf.d.ts` (the types of the API, for your editor) and an
`AGENTS.md` that tells a coding agent how plugins work: with no build step and a reload on each save, an agent can
write a plugin and see it work at once.

### A plugin's folder

```
word-stats/
  plugin.json
  main.ts
  fiveleaf.d.ts
```

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

Every id starts with the plugin's name and a dot. A title takes other languages as `title:<tag>`, for
**View → Language**. FiveLeaf's own prefixes, as `file`, `edit`, `view` and `git`, are reserved.

#### Activation

A plugin starts when something it names happens, and costs nothing before:

| Event                       | When                                                    |
| --------------------------- | ------------------------------------------------------- |
| `onCommand:<id>`            | One of its commands runs. Any command starts its plugin. |
| `onLanguage:<language>`     | A document of a language opens or comes to the front     |
| `workspaceContains:<glob>`  | The folder has a file the glob matches, as `go.mod`     |
| `onView:<id>`               | One of its views shows                                  |
| `onPreview:<id>`            | One of its previews shows                               |
| `onStartup`                 | FiveLeaf starts                                            |
| `*`                         | Anything above                                          |

Languages are the highlighter's names in lower case: `go`, `typescript`, `markdown`, `python`, `plaintext`, and the
languages plugins declare.

#### Contributions

| Key               | What it adds                                                                                   |
| ----------------- | ---------------------------------------------------------------------------------------------- |
| `commands`        | Commands in the command palette, the shortcuts editor and `ff run`. `arg` names what `ff run id=arg` gives. |
| `keybindings`     | Shortcuts in keybindings.json's format. A plugin only gets keys that are free; see below.      |
| `keymaps`         | Named shortcut presets, selected in **Settings → Keyboard Shortcuts → Keymap**                |
| `menus`           | `plugins` (the Plugins menu), `explorer/context` and `tab/context`, whose commands get the path |
| `settings`        | User settings under `plugins.<name>`. Types are `boolean`, `number`, `integer` or `string`. Use `enum` for a choice and `minimum` / `maximum` to bound integers. |
| `statusItems`     | Items of the status bar, `align` `left` or `right`, which the plugin fills                     |
| `views`           | Views in the sidebar, beside Explorer, Search and Source Control, which the plugin renders     |
| `previews`        | Previews beside the source of the files a `pattern` matches, `output` `svg`, `markdown` or `view` |
| `formatters`      | Formatters of `languages`, for **Format Document** and `formatOnSave`                          |
| `webviews`        | Panels of HTML from the plugin's folder, in the sidebar                                        |
| `themes`          | Themes, in FiveLeaf's format or VS Code's                                                         |
| `iconThemes`      | File and folder icon packs, selected in **Settings → Appearance → File Icons**                  |
| `languages`       | File `extensions` and `filenames`, the Chroma `lexer` that highlights them, and `lineComment`  |
| `snippets`        | Snippets in VS Code's format, for a `language` or `*`                                          |
| `terminalLinks`   | A `pattern` of links clicked in a terminal (URLs and paths), which runs `command` with the match |
| `languageServers` | Language servers: a `command`, its `args` and `initializationOptions`, for `languages`         |
| `debugAdapters`   | DAP servers for **Run and Debug** with language IDs, a transport and launch or attach arguments |
| `toolbox`         | A Toolbox tool: its `page`, `title` and searchable `features`                             |
| `agents`          | Coding agents whose sessions the **Sessions** view lists and resumes; see below                 |
| `agentServers`    | ACP/CLI agents for chat and roundtables: commands, environment and routing tags; no `main` needed |

#### Keyboard shortcuts

A plugin's shortcut takes a key only while no command of FiveLeaf's has it by default and keybindings.json does not bind
it, so installing a plugin never changes what a key you know does. A shortcut that is taken shows on the Plugins page
of the settings window, where **Use Plugin's Shortcut** writes it to keybindings.json. keybindings.json can bind any
plugin command to any key, as a FiveLeaf command.

#### Keymap presets

A keymap plugin adds a complete choice to **Settings → Keyboard Shortcuts → Keymap**. Create a folder named
`team-keys` with this `plugin.json`; it needs no `main` or JavaScript:

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

Run `ff plugin dev ./team-keys` to load that folder, or put it in `~/.fiveleaf/plugins/team-keys`.
Git repositories install with `ff plugin install owner/repo`, as other plugins do. Select **Team Editor** in
the Keymap menu, or set `"keymap": "team-keys.editor"` in `settings.json`.

Each preset needs a unique `id` beginning with the plugin name and a dot, a `label`, and a list of `bindings`.
`key` is the shortcut for all systems; `mac`, `win` and `linux` replace it on their respective system.
Separate strokes with spaces for a chord. `when` accepts FiveLeaf's boolean conditions, such as `editorTextFocus`,
`terminalFocus`, `!inputFocus`, `&&`, `||` and parentheses. `args` is passed to the command unchanged. Commands can be
FiveLeaf command IDs, its supported VS Code aliases, or enabled plugins' commands. See
[Keyboard shortcuts](SETTINGS.md#keybindingsjson) for the supported commands and conditions.

Selecting a preset applies its bindings immediately. Unspecified commands keep FiveLeaf's defaults, and personal
entries in `keybindings.json` take priority. Editing the plugin reloads its presets. Disabling or removing the selected
plugin restores the default keys while keeping its selected ID, so enabling it again restores the preset.
Install `keymap-vscode`, `keymap-jetbrains` or `keymap-sublime` to restore the corresponding FiveLeaf preset.
VS Code and JetBrains keymap names describe their key conventions; FiveLeaf plugins use the manifest above, not
those editors' extension packages.

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

A plugin may also export `activate()`, which runs as it starts, after its top level.

What a plugin's code has:

- The `fiveleaf` module, below. Its types are in `fiveleaf.d.ts`.
- `console`, whose lines go to the plugin's log.
- `fetch`, `Request`, `Response`, `Headers`, `URL`, `URLSearchParams`, the Streams, `AbortController`, `setTimeout`,
  `setInterval` and `queueMicrotask`.
- Imports of its own files by relative path, with or without their extension.

What it does not have: `require`, `node:` modules, npm packages at run time, `eval` and `new Function`. To use a
package, `npm install` it in the plugin's folder and run `ff plugin pack`, which copies the files of the ES module
build it imports into `vendor/` and points the imports at them. A package that is only CommonJS cannot run.

A file larger than 1 MiB is not loaded.

### The fiveleaf module

Positions are a `line` and a `character` in UTF-16 code units, both from 0, as JavaScript strings and LSP count them.
Every call that reaches the editor returns a promise.

| Namespace                | Calls                                                                                     |
| ------------------------ | ----------------------------------------------------------------------------------------- |
| `commands`               | `register(id, fn, {title})`, `run(id, arg)` (FiveLeaf's commands too), `list()`              |
| `editor`                 | `active()`, `visible()`; an editor's `document`, `selections`, `edit(edits, {version})`, `insert(text)`, `setSelections`, `reveal(line)` |
| a document               | `path`, `name`, `language`, `version`, `isDirty`, `lineCount`, `text(range?)`, `lineAt(line)`, `save()` |
| `workspace`              | `root()`, `isRemote()`, `open(path, {line})`, `readFile`, `writeFile`, `stat`, `findFiles(glob)`, `documents()` |
| `window`                 | `notify(message, {kind, description, action})`, `fail`, `pick(items)`, `prompt({title, value})`, `progress(title, fn)`, `openText(text)` |
| `settings`               | `get(key)` of the plugin's settings, `all()`, `fiveleaf(key)` of FiveLeaf's own, `onChange(fn)` |
| `storage`                | `get(key)`, `set(key, value)` and `keys()`, global or with `{folder: true}` of the folder |
| `events`                 | `on(name, fn)`; see below                                                                 |
| `process`                | `run(command, args, {cwd, input, timeout})`, on the folder's machine                      |
| `languages`              | `setDiagnostics(path, items)`, `clearDiagnostics(path?)`, `registerFormatter(id, fn)`, `registerHover(language, fn)` |
| `statusItem(id)`         | `set({text, tooltip, icon, command, onClick})`, `hide()`                                  |
| `views`                  | `render(id, tree)`, `onShow(id, fn)`, `reveal(id)`                                        |
| `previews`               | `register(id, fn)`, `refresh(id)`                                                         |
| `webview`                | `open(id)`, then the panel's `post(message)`, `onMessage(fn)`, `close()`                  |
| `agents`                 | `register(id, fn)`, the function that lists the sessions of an agent                      |
| `log`                    | `info`, `warn`, `error`, `debug`                                                          |

An edit names the version of the document it was worked out from. When the document changed since, as when you typed
meanwhile, `edit` returns `false` and changes nothing. An edit that applies is one step of undo.

#### Notifications

`window.notify()` shows a card in the active editor window using its current theme. The optional `kind` chooses
the icon and color. Use `info` (the default), `success`, `warning`, `error`, or `progress`. An optional `description`
appears below the message. `window.fail(message)` shows an error with the plugin's name.

```ts
const open = await window.notify("Report ready", {
  kind: "success",
  description: "The workspace report contains 12 findings.",
  action: "Open report",
});
if (open) await workspace.open("report.md");
```

With an action, the promise resolves to `true` when the user chooses it and `false` when the card times out or
is dismissed. Without an action, it resolves to `false` as soon as the notification is queued.

The status bar bell is always visible. A dot indicates unread notifications from the app or its plugins.
The bell keeps the latest 100 unread entries across restarts, including timed-out cards. Clicking an entry marks it
as read. The circled × removes one notification, and **Clear** removes all entries.

#### Events

| Event               | When                                              |
| ------------------- | ------------------------------------------------- |
| `editor.open`       | A document opens                                  |
| `editor.change`     | A document changed, once its edits rest for 0.3 s |
| `editor.focus`      | A document comes to the front                     |
| `editor.didSave`    | A document was saved, by hand or by auto save     |
| `editor.close`      | A document closed                                 |
| `workspace.changed` | The window's folder changed                       |
| `settings.changed`  | The settings changed                              |
| `theme.changed`     | The theme or the appearance changed               |

A document event has the document's `path`, `language`, `version`, and the `document` itself.

### Files and programs on SSH hosts

Plugins run on this machine. `workspace` and `process` reach the folder's machine, so `readFile("go.mod")` reads it on
the host and `process.run("gofmt", ["-l", "."])` runs there, in the folder, with nothing installed on the host for
the plugin. Each call takes a round trip there.

### Diagnostics

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

Diagnostics are underlined in the editor, as their severity: error, warning, info or hint. The status bar counts the
errors and warnings, a click shows them all in **Show Problems**, and the message of the one on the caret's line shows
beside it.

### Formatters

For a command-line formatter, declare the executable and its arguments in the manifest. FiveLeaf pipes the current
document into the command and applies its UTF-8 standard output on a successful exit. A failed command reports its
standard error. Arguments are passed directly, with `${file}` expanded to the native file path, `${tabSize}` to the
indent width and `${useTabs}` to `true` or `false`. The `command` field also accepts `${settings.key}` for a string
setting declared by the plugin. Commands run on the file’s machine in the workspace root, or its containing folder
when the file is outside the workspace.

Use `settingArgs` for optional flags. Each key names a declared setting and each value is one argument template.
FiveLeaf appends flags in key order for explicitly set values, including `false`. `${value}` inserts the typed
value literally, including spaces. An unset or `null` setting omits the entire argument, as does an empty text
value. Manifest defaults supply the values shown in Settings. Resetting a field removes its flag so the tool can
use its own configuration.

```jsonc
"settings": { "printWidth": { "type": "integer", "minimum": 1 } },
"formatters": [{
  "id": "prettier.format", "languages": ["typescript", "javascript", "css"],
  "command": "prettier", "args": ["--stdin-filepath", "${file}"],
  "settingArgs": { "printWidth": "--print-width=${value}" }
}]
```

For a JavaScript formatter, declare its ID and languages and register the ID in the entry module. A handler such as
`tidy.format` can return the formatted text, text edits or `null` to keep the current text.

```ts
languages.registerFormatter("tidy.format", async (doc) => (await doc.text()).trimEnd() + "\n");
```

**Selection → Format Document** (⇧⌥F) formats with it. With `formatOnSave` on for the language, saving by hand formats
first, for up to 1.5 seconds; past that, the file saves as it is. Auto save does not format.

### Views

A view is a tree of nodes that FiveLeaf draws in its theme, the zoom and the language, with no webview:

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

Nodes: `stack` and `row` with `children`, `text`, `markdown`, `code`, `svg`, `list`, `tree` (items with `children`,
`open` at first), `input` (`onChange`, `onSubmit`), `select`, `checkbox`, `button`, `progress` (a `value` from 0 to 1,
or a spinner without one), `divider` and `empty` (its `text` under an `icon`). A function in the tree, as `onClick`, is
called back in the plugin.
Icons are FiveLeaf's: `check`, `x`, `plus`, `file`, `folder`, `search`, `star`, `info`, `warning`, `error`, `trash`,
`refresh`, `settings`, `terminal`, `branch`, `commit`, `tag`, `clock`, `lightbulb`, `sparkles`, `comment`, `copy`,
`external`, `pencil`, `list`, `function`, `hash`, `lock`, `server`, `cloud`, `send`, `keyboard`, `box`, `braces`,
`download` and `image`.

### Previews

```jsonc
"previews": [{ "id": "d2.preview", "pattern": "*.d2", "output": "svg" }]
```

```ts
previews.register("d2.preview", async (text, { path }) => {
  const { stdout } = await process.run("d2", ["-", "-"], { input: text });
  return stdout; // SVG
});
```

**View → Toggle Preview** (⇧⌘V) and **Open Preview to the Side** show it, rendered again a moment after each edit.
Output `markdown` returns Markdown text, and `view` a tree of nodes.

### Webviews

For what nodes cannot draw, a webview shows an HTML file of the plugin's folder in the sidebar:

```jsonc
"webviews": [{ "id": "chart.panel", "title": "Chart", "path": "panel.html" }]
```

```ts
const panel = await webview.open("chart.panel");
panel.onMessage((m) => log.info("from the page", m));
panel.post({ points: [1, 2, 3] });
```

The page has `fiveleaf.post(message)` and `fiveleaf.onMessage(fn)`. On Linux, webviews need WebKitGTK.

### Themes

```jsonc
"themes": [{ "id": "night.dark", "label": "Night", "path": "night.json" }]
```

A theme's `label` is what the picker shows, and `path` is a file inside the plugin's folder: an absolute path, or
one that leaves the folder, is refused with the manifest. `ff plugin new <name> --theme` writes a theme plugin
with no code to start from.

A theme file is FiveLeaf's format, the colors of one of FiveLeaf's palettes changed:

```json
{
  "base": "dark",
  "colors": { "editor": "#1e1f29", "chrome": "#191a21", "accent": "#bd93f9", "selection": "#44475a80" },
  "syntax": { "keyword": "#ff79c6", "string": "#f1fa8c", "comment": "#6272a4" },
  "ansi": ["#21222c", "#ff5555", "#50fa7b", "#f1fa8c", "#bd93f9", "#ff79c6", "#8be9fd", "#f8f8f2"]
}
```

or a VS Code color theme as it is, whose `colors` and `tokenColors` FiveLeaf maps to its own. `base` is `dark`, `light`
or a theme's id, as `claude`. The theme shows in **View → Theme → Color Theme…**, the settings window, and the command
palette. A theme may live in `settings.json` under `themes`, without a plugin; see [THEMES.md](THEMES.md).

A theme file may also set `name`, `mono`, `sentence`, `card`, `grain`, `derived`, and `metrics` for the chrome's gaps,
radii, text and weights: see [THEMES.md](THEMES.md). A theme of your own needs no plugin: put it in
`~/.fiveleaf/themes`, and **Plugins → Open Themes Folder** opens that folder. A user theme with the same id as a plugin's
is the one shown.

### Icon themes

```jsonc
"iconThemes": [{ "id": "copper-icons.copper", "label": "Copper Icons", "path": "themes/copper.json" }]
```

An icon theme changes file and folder icons independently of the color theme. Its `id` starts with the plugin's
name and a dot, `label` is the name shown in settings, and `path` is a JSON file inside the plugin's folder.
Use relative paths with forward slashes; absolute paths, parent segments and links out of the plugin's folder are
refused. A plugin that contributes only icons needs no `main` or activation event.

Install, update, enable and remove icon plugins using the same plugin commands and settings page as other plugins.
Choose a pack in **Settings → Appearance → File Icons**. If its plugin is disabled or removed, FiveLeaf uses its
default icons while keeping the saved choice. Enabling or reinstalling the plugin restores that choice.

See [ICON-THEMES.md](ICON-THEMES.md) for the SVG pack format and a complete example. This format is FiveLeaf's own;
Zed and VS Code icon extensions need a FiveLeaf manifest and mappings before they can be installed.

### Languages and snippets

```jsonc
"languages": [{ "id": "d2", "extensions": [".d2"], "lexer": "yaml", "lineComment": "#" }],
"snippets": [{ "language": "d2", "path": "snippets.json" }]
```

`lexer` is a [Chroma](https://github.com/alecthomas/chroma) lexer's name. Snippets are VS Code's format, with tab stops,
placeholders, choices and variables as `TM_FILENAME` and `CURRENT_YEAR`; **Selection → Insert Snippet…** inserts one.

### Language servers

Install the language plugins you need in **Plugins → Manage Plugins…**, install each server program
on the workspace machine and enable its plugin. See [Language servers](LANGUAGE-SERVERS.md).

```jsonc
"languageServers": [{ "id": "gopls.server", "languages": ["go"], "command": "gopls" }]
```

A plugin of a few lines brings a language: FiveLeaf starts the server as a document of its languages opens, on the
folder's machine, so it runs on the SSH host of a remote folder. FiveLeaf shows the server's diagnostics, and asks it for
**Go → Go to Definition** (F12), **Go → Show Hover**, **Selection → Complete Word** (⌃Space in the editor), and
**Format Document** when no plugin formats the language.

A static server's `command` can reference a nonempty string with `${settings.key}`. It uses the user's value
or the manifest default. FiveLeaf passes `args` literally. For optional flags, use the same `settingArgs` syntax as a formatter.
FiveLeaf reads these settings on server startup and on **Restart**.

For servers that need setup, use `resolve` instead of `command`:

```jsonc
"settings": { "helperPath": { "type": "string", "default": "my-helper" } },
"languageServers": [{
  "id": "example.server", "languages": ["java"],
  "resolve": { "command": "${settings.helperPath}", "args": ["resolve"] }
}]
```

The resolver runs once at server startup, on the workspace's machine. Its stdin
is JSON `{root, settings}`: an absolute host path and the plugin's explicitly
stored settings (without manifest defaults). `${settings.key}` references in the
resolver command and arguments use stored settings with manifest defaults as a
fallback; substitution passes literal arguments and never invokes a shell.
Keep resolution fast and offline; dependency installation belongs in a separate
setup command. Resolution and initialization share a 30-second deadline.

The resolver must print one JSON object and exit successfully:

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

FiveLeaf starts the returned command directly. `env` overrides inherited variables;
`unsetEnv` removes them on the workspace host. `configuration` answers
`workspace/configuration` requests by dotted section. `workspaceFolders` is used
for initialization and folder requests; the first folder supplies `rootUri`.
Folders and mappings must use absolute `file:` URIs. URI prefix mappings run in
both directions and match whole path components, longest first. Mapping follows
known LSP document, workspace, definition and diagnostic URI fields, including
keys in workspace edits. Source/edit text, hover content, configuration,
`initializationOptions` and unknown extension payloads pass through unchanged.
Plugins prepare server-specific paths inside their own configuration. This lets
a server import canonical paths and the editor keep the workspace's symlink
aliases for definitions and diagnostics. Standard declarations remain valid.

### Debug adapters

To try a debug adapter plugin with a new file extension, generate and link the example.

```sh
ff plugin new sample-debug --debug-adapter
cd sample-debug
ff plugin dev .
```

The example uses Python syntax in `example.sample-debug`. Install `debugpy` in a Python environment with
`python -m pip install debugpy`, then enter that interpreter's path under **Settings → Plugins → Sample debug**.
Open the generated folder as a project. Toggle a breakpoint on line 2 using the editor gutter or `F9`, then press
`F5` to see the value of `message` in **Run and Debug**. The installed plugin's details list its adapter under
**Debug adapters**.

Hover to the left of a line number to preview the breakpoint. Click to add it and click again to remove it.
During a session, a filled dot marks a breakpoint confirmed by the adapter. Hover over a hollow dot to see
its verification status. An arrow beside the dot shows the paused line.

To use your own server, change the language association, executable and launch arguments in `plugin.json`.
You can implement the server in any language. The example below starts a packaged Node.js adapter that implements
the [Debug Adapter Protocol](https://microsoft.github.io/debug-adapter-protocol/).

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

`languages` inside the adapter names the language IDs it handles. For a language already recognized by FiveLeaf,
use its existing ID. For a new language, declare its extensions or filenames in `contributes.languages` as shown.
When multiple enabled adapters handle the active language, `F5` lets the user choose one.
The contribution `id` starts with the plugin's name and a dot. `adapterID` sets the ID sent in the DAP `initialize`
request and defaults to the contribution `id`. The plugin can supply `initializationOptions` for adapters that read it.

Standard I/O is the default transport. The adapter reads framed DAP requests from stdin and writes framed messages to stdout.
For a TCP adapter, set `transport.kind` to `tcp` and choose one way to discover its port.

| Adapter | Manifest fields |
| --- | --- |
| Prints the port it chose | `"command": "dlv", "args": ["dap"], "transport": { "kind": "tcp", "ready": "DAP server listening at: " }` |
| Already listening at a fixed address | `"transport": { "kind": "tcp", "address": "127.0.0.1:4711" }` |

With a fixed address, omit `command` to connect directly. When the plugin supplies a command, FiveLeaf starts the
process and owns it for the session. A DAP `attach` request is a separate choice. For `F5`, FiveLeaf uses the
`launch` object when present, or the `attach` object when it is the only one declared. `"request": "attach"` in
`launch` also selects `attach`. Put the arguments required by the target adapter in the selected object.

Commands, arguments, environment values and strings inside `launch` or `attach` can use `${settings.key}` for a
string setting, or `${file}`, `${workspaceFolder}`, `${fileDirname}` and `${fileBasename}` for paths. Each item
of `args` stays a separate process argument. In a local project, `${pluginDir}` locates files in the installed
plugin. For an SSH workspace, install the adapter and any helper on the workspace host and refer to its executable
by name on the remote `PATH` or by absolute host path.

TCP adapters can send the standard `startDebugging` reverse request to open a child target on the same server.
FiveLeaf sends the child's configuration, applies source breakpoints to it and shows the stack and variables of
the paused target. The JavaScript and TypeScript plugin uses this for Node.js. Adapters that launch a program
in the editor use the internal debug console. For a TCP process started over SSH, prefer a `ready` message so
the adapter can announce when its listener accepts connections.

#### Resolving a launch at runtime

A `resolve` helper can find a workspace-specific executable, port or launch arguments. For example, add
`"resolve": { "command": "node", "args": ["${pluginDir}/resolve.mjs"] }` to the adapter in the declaration above.
The helper reads one JSON object from stdin with `root`, `file` and the plugin's explicitly stored `settings`.
The helper writes one JSON object to stdout and exits. For example, this reply supplies arguments for the session.

```json
{ "env": { "ACME_MODE": "debug" }, "launch": { "entryPoint": "main" } }
```

Omitted fields keep their manifest values. `env` and `launch` override individual keys. Returned `command`, `args`,
`transport` and `initializationOptions` replace those fields. A resolver can also return the entire adapter configuration.
Write diagnostics to stderr. Resolution and the adapter's startup handshake share a 30-second timeout.

### Coding agents

The **Sessions** view lists the sessions of Claude Code, Qoder, Codex and Grok. A plugin adds another agent, whose
sessions show there with its icon. **Resume** opens the transcript in a chat in the editor.

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

`sessions` reads each file the glob matches (`~` is the home folder, `**` any folders) as `json`, one object, or
`jsonl`, an object a line. Each field is a path of keys by dots, as `meta.cwd` or `messages.0.content`; of a `jsonl`
file each takes the first line that has it, but `title` and `updated` the last. A session's id is the file's name
without its extension unless `id` names one, and its time the file's unless `updated` names one: milliseconds or
seconds since 1970, or ISO 8601 text. A file is read again only once it changes.

An agent without `sessions` lists them with a function, which starts the plugin as the view reads:

```ts
import { agents, workspace } from "fiveleaf";

agents.register("pi.agent", async () => {
  const index = JSON.parse(await workspace.readFile("/Users/me/.pi/index.json"));
  return index.map((s) => ({ id: s.id, title: s.name, folder: s.cwd, updated: s.mtime, prompt: s.firstPrompt }));
});
```

FiveLeaf calls it off its main thread each time it reads the sessions, every few seconds while the view shows, and waits
1.5 seconds for it: what comes later shows a moment after, and a function that runs 10 seconds is given up. One that
fails keeps the sessions it gave last, and its error goes to the plugin's log, once until it changes. A plugin turned
off takes its agents and their sessions with it.

### ACP agent servers

The **Agent** view connects to agents using the [Agent Client Protocol](https://agentclientprotocol.com/)
or runs one-shot CLI commands. Both can participate in a separate roundtable window.
An agent server declaration needs no JavaScript entry point or session scanner:

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

Each ID must start with the plugin name and a dot, and be unique within its agent server declarations.
`command` is an executable, not a shell command; arguments go in `args`. A bare command is found on `PATH`;
a relative path such as `./bin/agent` is resolved from the plugin's folder. The agent runs in the current
project folder by default. `env` extends the environment inherited by FiveLeaf. Enabling the plugin makes the agent
available; connecting in **Agent** starts ACP, while CLI runs on each message. Turning the plugin off removes that provider.

For a launcher such as `npx`, set `localCommand` to the agent CLI that must also be installed.
FiveLeaf checks both executables before enabling the plugin. `localCommandEnv` can name an environment
variable with the agent's executable path, such as `CODEX_PATH` for Codex.

Optional fields match the user-level [agent settings](SETTINGS.md#acp-agents): `protocol` (`acp` by default,
or `cli`), `workingDirectory`, `capabilities`, `disabled`, `timeoutSeconds` (default 120) and CLI-only
`promptArgument`. Relative working directories resolve from the project; they do not change the ACP session
or editor file boundary. Tags such as `code`, `reasoning` and `search` choose the next roundtable speaker
and grant no permissions. CLI receives its prompt on stdin unless `promptArgument` is true, which appends
it as one final argument. Commands must run non-interactively and exit after replying.

Install and enable an agent in **Settings → Plugins**, then select it in chat.

| Plugin | Agent | Launch command |
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

The `npx` presets need Node.js and download their pinned package on first connection.
The other plugins use the installed agent's native ACP command. Sign in through its CLI
before connecting. See [Agent setup](AGENTS-ACP.md#agent-plugins) for runtime and account requirements.

Users can override a plugin's agent definition, including its tags or enabled state, in Agent settings.

For a local executable or another ACP/CLI agent, choose **Configure Agents… → Add Custom Agent…**
or use the user-level [`agentServers` setting](SETTINGS.md#acp-agents). Existing `contributes.agents` declarations keep their
**Sessions** listing and **Copy Resume Command** support. A plugin may provide both kinds of contribution.

### User scripts

A `.ts` or `.js` file in `~/.fiveleaf/scripts` is a plugin without a manifest: each function it exports is a command,
`script.<file>.<function>`, titled from its name, in the command palette, the shortcuts editor and `ff run`. A
function gets what `ff run script.text.upperCase=hello` gives, or nothing from the palette.
**Plugins → Open Scripts Folder** makes the folder with an example.

**Plugins → Run Selection as Script** runs the selection, or the whole file, once, as a plugin of its own: its
`console.log` goes to the log, and what its default export returns shows in a toast.

## Sharing a plugin

Put `plugin.json` at the repository root or in a plugin subdirectory. Keep the selected directory and its files free
of symbolic links. Use the manifest's `name` as the index entry's `name` and add the entry to `index.json` by pull
request in [YancyBuilders/fiveleaf-plugins](https://github.com/YancyBuilders/fiveleaf-plugins).

```json
{ "name": "material-icons", "displayName": "Material Icon Theme", "repository": "https://github.com/YancyBuilders/fiveleaf-plugins.git#material-icons", "description": "Material Icon Theme artwork for files and folders in FiveLeaf.", "tags": ["icon-themes"] }
```

For an official release, publish the plugin directories, license, gallery README and `index.json` from `extensions/`
at the root of the plugin repository's `main` branch. The Go catalog loader and tests belong to the FiveLeaf client.
Users moving from `fiveleaf-themes` or `fiveleaf-icons` can install the individual plugins they use, then remove the
old bundles. FiveLeaf recognizes their saved selections.

Tags such as `language`, `themes`, `icon-themes`, `lsp`, `agent`, `snippet`, `formatter` and `keymap`
place index entries in the corresponding catalog filters. Installed plugins use the contributions in their manifests.
The **Languages** filter covers both `language` and `lsp` tags, and installed plugins that declare
either contribution. The **Debug adapters** and **MCP servers** filters appear when the index contains
`debug-adapter` or `mcp-server` tags, or when an installed plugin declares that contribution.
