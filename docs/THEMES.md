# Themes

A theme is a JSON file, or JSONC with comments: FiveLeaf's own format, or a VS Code color theme, whose colors and token
colors FiveLeaf maps to its own. It paints the chrome, the code and the terminal.

FiveLeaf includes one color theme with light and dark appearances. Its rounded sidebar sits against the same
background as the editor, title bar, tabs and status bar. Search **Settings → Plugins** for a theme by name,
or run `ff plugin install one-dark-pro` to install One Dark Pro. Each adaptive theme includes both palettes.
When a selected plugin theme is
unavailable, the window and terminal use FiveLeaf until the saved theme is available again.

**Settings → Theme → Color Theme… → One Dark Pro** selects One Dark Pro. Its editor
highlights, Git gutter marks and 16 terminal colors follow the default palette from the
[One Dark Pro VS Code extension 3.20.6](https://marketplace.visualstudio.com/items?itemName=zhuangtongfa.Material-theme).
JavaScript and TypeScript use blue functions, red properties, yellow constant bindings and purple template
delimiters, with italic comments and parameter declarations. The title, tab and status bars share the editor's
charcoal surface; the sidebar stays darker, with a muted gray selection. Go uses red variables, yellow named
types and assignments, purple built-in types, blue built-in functions and italic parameter declarations.
FiveLeaf maps shared token categories rather than every
language-specific VS Code scope. The reference is Copyright (c) 2013–2022 Binaryify, under the
[MIT license](../cmd/fiveleaf/testdata/onedarkpro/LICENSE.txt).
It stays dark regardless of appearance. Use `"theme": "onedarkpro"` in `settings.json`, or `"base": "onedarkpro"`
in a custom theme to build on it.

**View → Theme → Ayu** selects Ayu's warm accents and light or dark editor colors. It follows **Appearance**:
light, dark, or the system. Use `"theme": "ayu"` in `settings.json`, or `"base": "ayu"` in a custom theme to build
on it. The palettes use [Ayu colors 9.1.0](https://github.com/ayu-theme/ayu-colors/tree/c6e86054896ef10c69ba899dc11fdad0e004d4d3),
Copyright (c) Konstantin Pschera, under the [MIT license](https://github.com/ayu-theme/ayu-colors/blob/c6e86054896ef10c69ba899dc11fdad0e004d4d3/license).

**Settings → Appearance → Mica Material** adds a frosted surface over slow, curling bands of color.
The light appearance blends the selected hue into pearl white. The dark appearance draws it out of deep shadow.
Tabs, notifications, popovers and roundtables share the window’s current. The sidebar and chat input sit
just above the compact status bar. Roundtable messages, member lists and the execution graph use the same frost.
Particles gather into the FiveLeaf icon as soon as an Agent chat or roundtable conversation opens.
The particles are silver in dark mode and gray in light mode. Moving the pointer shifts the highlight.
As you pass over a leaf, nearby particles scatter and settle back into its outline.
Its frost, hue, intensity and speed can be adjusted there, with a live preview while dragging.
The theme keeps its text, syntax and terminal colors. **Animated Mica Background** and the system's
Reduce Motion setting can hold the background still. **Background Opacity** also reveals the desktop
on systems that support it; the colored background works without a native window material.

## Your own themes

Put a file in the themes folder, one theme each:

| System  | Folder                                  |
| ------- | --------------------------------------- |
| macOS   | `~/.fiveleaf/themes`                       |
| Linux   | `~/.fiveleaf/themes`                       |
| Windows | `%USERPROFILE%\.fiveleaf\themes`            |

With `FIVELEAF_DATA_DIR` set, the folder is `$FIVELEAF_DATA_DIR/themes` instead. **Plugins → Open Themes Folder** opens it,
and writes an example the first time.

The file's name without `.json` or `.jsonc` is the theme's id, as `"theme": "paper"` in `settings.json`. Its `name` is
what the theme picker shows, and defaults to the id. A theme file is loaded again as it is saved, as a plugin's is.

```jsonc
{
  "name": "Paper",
  "base": "light",
  "mono": false,
  "sentence": false,
  "colors": {
    "editor": "#fdfcfa",
    "chrome": "#f5f4f1",
    "text": "#1a1918",
    "accent": "#c4613f",
    "selection": "#5769f728"
  },
  "syntax": {
    "keyword": "#4c5ee0",
    "string": "#2c7a39",
    "comment": "#87857f"
  },
  "metrics": {
    "gap": 4,
    "radius": 6,
    "text": { "body": 12.5, "title": 12.5 }
  }
}
```

## In settings.json

A theme may be written in `settings.json` too, under `themes`, by its id. `theme` chooses it, and another theme's
`base` may name it. These are read as the files are, and a save applies them at once:

```json
{
  "theme": "paper",
  "themes": {
    "paper": {
      "name": "Paper",
      "base": "light",
      "colors": { "editor": "#fdfcfa", "accent": "#c4613f" },
      "syntax": { "keyword": "#4c5ee0" }
    }
  }
}
```

These travel with `settings.json`. `fiveleaf` and `mayoi` select the built-in theme. The original alternate theme
IDs, including `matrix`, select the corresponding official plugins. For example, `onedarkpro`,
`fiveleaf-themes.onedarkpro` and `one-dark-pro.theme` all select One Dark Pro.

## Fields

| Key       | What it does                                                                       |
| --------- | ---------------------------------------------------------------------------------- |
| `name`    | The name in the theme picker.                                                       |
| `base`    | The palette it changes: `"dark"`, `"light"`, or a theme's id, as `"claude"` or `"paper"`. |
| `dark`    | `true` for a theme that is always dark, whatever the appearance; `false` for one that is always light. |
| `mono`    | `true` draws the whole window in the code's font.                                   |
| `sentence`| `true` titles the sidebar and panel in sentence case, as `Source control`.          |
| `card`    | The radius of the card the editors and panel sit in, `0` for none.                  |
| `grain`   | How grained that shell is, from `0` to `1`.                                         |
| `derived` | `true`, the default, makes the legible and hover colors from the palette's; `false` keeps the base's. |
| `colors`  | The palette's colors, by FiveLeaf's names below or VS Code's names with dots.          |
| `syntax`  | The colors of code, by kind.                                                        |
| `ansi`    | The terminal's 16 colors, black to bright white.                                    |
| `metrics` | The chrome's gaps, radii, text sizes and weights, and the heights of a row and a control. |
| `type`    | VS Code's `"light"`, `"dark"`, `"hcLight"` or `"hc"`, for a theme from VS Code.      |
| `tokenColors` | VS Code's token colors, mapped to FiveLeaf's kinds of code.                        |

A theme follows the appearance unless its `dark`, its `type` or its base does not: a theme based on The Matrix stays
dark, and one based on a light theme stays light.

VS Code's `colors` names include `editor.background`, `sideBar.background`, `titleBar.activeBackground`, `statusBar.background`, `focusBorder` and
`terminal.ansiRed`; the whole mapping is in `plugintheme.go`.

### Colors

`colors` takes `#rgb`, `#rrggbb` or `#rrggbbaa`. These names are FiveLeaf's own:

`editor`, `chrome`, `titlebar`, `panel`, `border`, `hover`, `active`, `text`, `muted`, `faint`, `accent`, `accentDim`, `fill`,
`fillText`, `dangerFill`, `dangerText`, `surface`, `surfaceHover`, `surfacePressed`, `scrollbar`, `inverse`,
`inverseText`, `line`, `selection`, `gutter`, `gutterNow`, `status`, `statusFg`, `modified`, `danger`, `match`,
`matchNow`, `matchRing`, `guide`, `guideNow`, `bracket`, `word`, `shadow`, `added`, `changed`, `removed`, `strip`.

`syntax` takes `plain`, `keyword`, `type`, `function`, `string`, `number`, `comment`, `operator`, `punctuation`,
`constant`, `builtin`, `tag`, `attribute`, `variable`, `heading`, `deleted`, `inserted`, `parameter`, `binding`,
`property`, `interpolation`, `bracket`, `object`, `builtinType`, `builtinFunction` and `assignment`. Additional
roles fall back to existing syntax colors when a theme does not set them. VS Code token settings also accept
`fontStyle: "italic"`; an empty
`fontStyle` clears inherited italics for that token category.

### Metrics

`metrics` takes `gap`, `pad`, `inset`, `radius`, `radiusField`, `radiusCard`, `radiusFull`, `textMicro`, `textSmall`,
`textBody`, `textTitle`, `weightMedium`, `weightBold`, `row` and `control`. `text` is `textBody` and `textTitle` at
once: a number, or `{"micro": 9.5, "small": 11.5, "body": 12.5, "title": 12.5}`. A key you leave out keeps the base's.

The numbers are bounded so that one theme cannot make the window unusable. A theme outside these bounds is left
out, as one that does not parse:

| Key | From | To |
| --- | ---: | ---: |
| `gap` | 1 | 24 |
| `pad` | 0 | 40 |
| `inset` | 0 | 48 |
| `radius`, `radiusField` | 0 | 24 |
| `radiusCard` | 0 | 40 |
| `radiusFull` | 0 | 1000 |
| `textMicro`, `textSmall`, `textBody`, `textTitle`, `text` | 6 | 28 |
| `weightMedium`, `weightBold` | 100 | 900 |
| `row` | 12 | 64 |
| `control` | 14 | 64 |

`card` is from `0` to `40`, and `grain` from `0` to `1`.

## From a plugin

A plugin contributes a theme in its `plugin.json`, and the file sits in its folder:

```jsonc
"contributes": {
  "themes": [{ "id": "night.dark", "label": "Night", "path": "night.json" }]
}
```

The same theme format applies. A theme's id must start with the plugin's name and a dot. `"dark": true` in the
contribution marks a theme that is always dark, and a theme's `base` may be another plugin's id or a theme of the
user's. A theme file must be inside the plugin's folder, and a link in it may not point out. See
[PLUGINS.md](PLUGINS.md#themes).

For a theme that follows the window, set `path` to the light palette and `darkPath` to the dark palette in the same
contribution. Both use the same ID. Optional `labels` and `descriptions` maps accept locale keys such as `en`, `ja`
and `zh-Hans`. A palette can set `syntaxItalic` by syntax role and `colors.rule` and `colors.gilt` for decorative lines.

`ff plugin new midnight --theme` writes such a plugin with a theme file, and no code. `ff plugin dev` links it
into the running FiveLeaf, and each save of the theme shows at once.

## When a theme cannot be read

A file that does not parse, a `base` that does not name a theme, and a chain of bases that leads back to itself are
left out of the theme picker and the `view.theme.<id>` commands. If `settings.json` still chooses one, FiveLeaf paints
its own light or dark theme.

No theme from a file or from `settings.json` can take an id of FiveLeaf's own, as `fiveleaf`, `matrix` or `claude`. One
id names one theme: `settings.json` is shown before the themes folder, which is shown before a plugin's.

## VS Code themes

A VS Code theme file works as it is. FiveLeaf reads `type`, `colors` and `tokenColors`, and maps their names to its own:
`editor.background` to `editor`, `focusBorder` to `accent`, `terminal.ansiRed` to the terminal's red, and so on.
The mapping is in `vscodeColors` and `ansiNames` in `cmd/fiveleaf/plugintheme.go`.

## How it fits together

`paletteFor` uses the built-in FiveLeaf palettes for the default theme, including in safe mode and when a selected
theme is missing. It resolves additional themes through the active plugins, theme files and settings. `derive` computes legible text and control states, and
`applyTheme` gives the resulting palette to the window. See [ARCHITECTURE.md](ARCHITECTURE.md#theme).
