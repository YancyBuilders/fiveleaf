# A FiveLeaf plugin

This folder is a plugin for FiveLeaf, a fork of [Mayoi](https://github.com/sorrycc/Mayoi), a code editor written in Go. Notes for
agents and people working on it.

## How it runs

- `plugin.json` declares the plugin: its `name`, its entry module `main`, when it starts (`activation`) and what it
  adds (`contributes`: commands, keybindings, keymaps, menus, settings, status items, views, previews, formatters, themes,
  languages, snippets, coding agents for the Sessions view and ACP agent servers for the Agent view).
  Prefix command and debug adapter contribution IDs with the plugin's name and a dot.
- `ff plugin new name --debug-adapter` writes a runnable example. `contributes.debugAdapters` associates an executable
  or TCP endpoint with language IDs. Declare a new file extension or filename in `contributes.languages` and use
  the same language ID in the adapter. The optional `adapterID` is the value sent in the DAP `initialize` request.
- `contributes.agentServers` declares ACP providers with `id`, `name`, `command`, optional `args` and `env`.
  They need no `main`. Commands use an executable and separate arguments, without a shell. A bare command is
  found on `PATH`; a relative executable path is resolved in the plugin folder. The process runs in the open
  project folder after the user connects. `contributes.agents` still lists and resumes terminal sessions.
- `contributes.keymaps` adds presets with `id`, `label` and `bindings`; each binding takes `key`, optional `mac`, `win`,
  `linux`, `when` and `args`, and a `command`. A keymap-only plugin needs no `main`. Users choose it under
  **Settings → Keyboard Shortcuts → Keymap**; their `keybindings.json` entries take priority.
- `contributes.toolbox` adds an editor tab with a tool's `id`, `title`, HTML `page` and searchable `features`.
  The page may load `app.js` and `style.css` beside it, and call `window.mygo.call("Toolbox.Copy", text)` and
  `Toolbox.Theme` for the palette. Use `ff plugin new name --tool` for plain HTML or `--tool=vue` / `--tool=react`
  for a Vite template. Build with `npm run build` and keep `dist/` in published framework plugins.
- `main.ts` runs in FiveLeaf's own JavaScript engine (moejs), not Node. TypeScript runs as it is, with no build step.
- The API is the `fiveleaf` module; its types are in `fiveleaf.d.ts`. Every call that touches the editor returns a promise.
- There is no `require`, no `node:` module and no npm package at run time. Import relative files of this folder. To use
  a package, run `ff plugin pack .`, which copies what the code imports into `vendor/`.
- `fetch`, `URL`, `setTimeout` and the Streams are there; `eval` and `new Function` are not.
- Files and programs go through `workspace` and `process`, which work on SSH folders too.

## Try it

```sh
ff plugin dev .      # load this folder into FiveLeaf, and again on each save
ff run <command-id>  # run one of its commands
```

Logs from `console.log` and errors show in FiveLeaf under **Plugins: Show Log**.

## Limits

A command's JavaScript that runs for 10 seconds without a break is stopped, and so is a plugin that keeps more than
256 MiB. A plugin that stops 3 times in a row is off until **Plugins: Reload Plugins**. A file larger than 1 MiB is
not loaded.
