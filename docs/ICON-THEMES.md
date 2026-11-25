# Icon themes

Open **Settings → Theme → File Icon Theme…** in the top-right corner to search the available icon packs. Use `↑` and `↓` to
preview their file and folder icons in the current window. Press `Enter` or click a pack to save it; `Esc` or a
click outside the picker cancels the preview. You can also choose icons in **Settings → Appearance → File Icons**.
The choice applies independently of the color theme, so a light or dark window can use the same icon pack.

Search **Settings → Plugins** for the icon set you want, or run `ff plugin install material-icons` for Material Icon Theme.
FiveLeaf File Icons, Catppuccin Latte Icons, Catppuccin Mocha Icons, Seti Icons and Ayu Icons also install individually.
Catppuccin's flavors share the same artwork: Latte has darker colors suited to light backgrounds, while Mocha has
lighter colors suited to dark backgrounds.
The `iconTheme` setting accepts a short name such as `material`, the old `fiveleaf-icons.material` ID or the plugin's `material-icons.icons` ID.

```json
{
  "theme": "onedarkpro",
  "iconTheme": "catppuccin-mocha"
}
```

| Choice | `iconTheme` |
| --- | --- |
| FiveLeaf | `""` |
| Catppuccin Latte | `"catppuccin-latte"` |
| Catppuccin Mocha | `"catppuccin-mocha"` |
| Material Icon Theme | `"material"` |
| Seti | `"seti"` |
| Ayu | `"ayu"` |

## Install a pack

Open **Plugins → Manage Plugins…** and install a FiveLeaf icon plugin from its Git URL, or run:

```sh
ff plugin install https://git.example.com/me/copper-icons.git
```

Its icon themes appear in the top-right Settings menu and Appearance settings after installation. A plugin may
contain several packs, such as light and dark variants. Update, disable or remove it through the Plugins settings
page or the usual `ff plugin` commands.

If a selected pack is missing, disabled or cannot be read, FiveLeaf uses basic file and folder symbols and retains the selected
ID. The choice takes effect again when the plugin is available. Turning a plugin off does not change the color
theme. File types without a mapping use the pack's default file icon. Invalid packs are omitted from the choices;
**Plugins → Show Plugin Log** shows the loading error.

## Write a pack

An icon plugin needs only a manifest, a JSON pack and SVG files. No JavaScript, activation event, npm package or
build step is required.

```text
copper-icons/
  plugin.json
  themes/
    copper.json
    icons/
      file.svg
      markdown.svg
      folder.svg
```

`plugin.json`:

```json
{
  "name": "copper-icons",
  "displayName": "Copper Icons",
  "version": "1.0.0",
  "description": "Copper file and folder icons",
  "contributes": {
    "iconThemes": [
      {
        "id": "copper-icons.copper",
        "label": "Copper Icons",
        "path": "themes/copper.json"
      }
    ]
  }
}
```

Every theme ID starts with the plugin's name and a dot and is unique within that plugin. Its label must not be
empty. The manifest's `path` is relative to the plugin folder; SVG paths in the pack are relative to the JSON
file's folder.

`themes/copper.json`:

```json
{
  "file": "file",
  "folder": "folder",
  "folderOpen": "folder",
  "fileNames": {
    "README.md": "markdown"
  },
  "fileExtensions": {
    "md": "markdown",
    "mdx": "markdown"
  },
  "folderNames": {
    "docs": "folder"
  },
  "folderNamesOpen": {
    "docs": "folder"
  },
  "icons": {
    "file": "icons/file.svg",
    "markdown": "icons/markdown.svg",
    "folder": "icons/folder.svg"
  }
}
```

The `icons` object gives each SVG a local ID. The other fields refer to those IDs:

| Field | Selects |
| --- | --- |
| `file` | Required default file icon |
| `folder` | Required default closed folder icon |
| `folderOpen` | Default open folder icon; falls back to `folder` |
| `fileNames` | Icons by filename, including its extension |
| `fileExtensions` | Icons by extension, written without a leading dot |
| `folderNames` | Closed folder icons by folder name |
| `folderNamesOpen` | Open folder icons by folder name |

Names and extensions match without regard to case. Exact filenames take precedence over extensions. The longest
matching extension wins, so `d.ts` can use a different icon from `ts`. A leading dot in an extension is optional.
Keys that become duplicates after normalization, such as `TS` and `.ts`, are rejected.

An open folder uses its `folderNamesOpen` mapping, then its `folderNames` mapping, then `folderOpen`. A closed
folder uses its `folderNames` mapping or `folder`. Every referenced icon ID must exist in `icons`.

`themes/icons/file.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <path fill="#c98b64" d="M3 1h6l4 4v10H3z"/>
  <path fill="#f5d5bd" d="M9 1v4h4"/>
</svg>
```

`themes/icons/markdown.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <rect x="1" y="3" width="14" height="10" rx="2" fill="#c98b64"/>
  <path d="M4 10V6l2 2 2-2v4m3-4v4m-1-1 1 1 1-1" fill="none" stroke="#fff4eb"/>
</svg>
```

`themes/icons/folder.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <path fill="#a96943" d="M1 3h5l2 2h7v9H1z"/>
  <path fill="#d6a17d" d="M1 6h14v8H1z"/>
</svg>
```

Use explicit SVG colors to preserve multicolor artwork; `currentColor` uses the window's text color in every view.
Include a `viewBox` and keep the drawing legible at small
sizes. All files must stay inside the plugin folder, including through symbolic links. Use relative paths with
forward slashes; external URLs and absolute paths are not accepted.

A pack supports up to 2,048 icons. The JSON file and each SVG are limited to 1 MiB each, and SVG assets to 32 MiB
in total. Each SVG needs positive dimensions of at most 4,096 by 4,096. A missing or invalid SVG makes the whole
pack unavailable until it is fixed.

To try a local pack:

```sh
ff plugin dev ./copper-icons
```

Select **Copper Icons** in Appearance. Saving the manifest, pack or SVG files reloads the plugin. Distribute the
folder as a Git repository, including the licenses and attribution required by any artwork you reuse.

FiveLeaf's icon format is distinct from Zed and VS Code extensions. Existing SVG artwork can be reused when its
license permits, with a FiveLeaf manifest and mappings.
