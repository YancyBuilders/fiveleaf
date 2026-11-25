package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/workspace"
	"fiveleaf/locales"
)

// pickItem is an item of a plugin's pick box.
type pickItem struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Icon        string `json:"icon,omitempty"`

	// icon is the icon Icon names, found as the box opens.
	icon *ui.SVG
}

// pluginPicker is the box a plugin's window.pick shows, found by label as
// quick open finds files.
type pluginPicker struct {
	open        bool
	query       string
	sel         int
	moved       bool
	items       []pickItem
	labels      []string
	placeholder string
	results     []workspace.Match
	shown       string
	stale       bool
	shook       time.Time
	done        func(i int) // the item's index, -1 for none
}

// openPluginPicker shows a plugin's items; done gets the index of the one
// chosen, or -1. A box already open answers -1 first.
func (a *app) openPluginPicker(items []pickItem, placeholder string, done func(int)) {
	a.closeIconThemePicker(false)
	if pk := &a.pluginPick; pk.open && pk.done != nil {
		pk.done(-1)
	}
	labels := make([]string, len(items))
	for i, it := range items {
		if it.Icon != "" {
			items[i].icon = namedIcon(it.Icon)
		}
		labels[i] = it.Label
		if it.Description != "" {
			labels[i] += " " + it.Description
		}
	}
	a.pluginPick = pluginPicker{open: true, stale: true, items: items, labels: labels, placeholder: placeholder, done: done}
}

func (a *app) closePluginPicker(chosen int) {
	pk := &a.pluginPick
	if !pk.open {
		return
	}
	done := pk.done
	pk.open, pk.done = false, nil
	if a.focus != paneTerminal {
		a.focusColumn()
	}
	if done != nil {
		done(chosen)
	}
}

func (a *app) pluginPickerView(c *ui.Context) {
	pk := &a.pluginPick
	if !pk.open {
		return
	}
	if pk.stale || pk.shown != pk.query {
		q := strings.TrimSpace(pk.query)
		if q == "" {
			pk.results = pk.results[:0]
			for i := range pk.items {
				pk.results = append(pk.results, workspace.Match{Index: i})
			}
		} else {
			pk.results = workspace.Fuzzy(pk.labels, q, 200)
		}
		pk.shown, pk.stale, pk.sel = pk.query, false, 0
	}
	// Return on nothing found shakes the box, as the palette does.
	choose := func(i int) {
		if i < 0 || i >= len(pk.results) {
			pk.shook = c.Now()
			return
		}
		a.closePluginPicker(pk.results[i].Index)
	}
	prompt := pickerPrompt{label: pk.placeholder, prompt: pk.placeholder, value: &pk.query, shook: pk.shook, close: func() { a.closePluginPicker(-1) }}
	accept := func() {
		sel := pk.sel
		a.post(func() { choose(sel) })
	}
	a.pickerDialog(c.Key("plugin-picker"), prompt, func(in ui.Element) {
		pickerNavigate(in, &pk.sel, &pk.moved, len(pk.results))
		if in.Submitted() {
			accept()
		}
	}, func() {
		if len(pk.results) == 0 {
			quickMessage(c, iconPlug, locales.PluginPickNoMatches())
		} else {
			a.pickerScroll(c.Key("plugin-pick-results")).Children(func() {
				for i, m := range pk.results {
					a.pluginPickRow(c, i, m)
				}
			})
		}
		pk.moved = false
		a.pickerActions(c, pickerBar{action: locales.QuickChooseAction(), enabled: len(pk.results) > 0, choose: accept, close: prompt.close})
	})
}

// pluginPickRow is an item of a plugin's pick box, the i-th found, as a
// command of the palette is: its icon, its label with the characters the
// query matched in the accent color, what it says beside it, and its
// detail below.
func (a *app) pluginPickRow(c *ui.Context, i int, m workspace.Match) {
	p := a.pal
	pk := &a.pluginPick
	it := &pk.items[m.Index]
	row := ui.ButtonBase(c.Key(m.Index)).MinHeight(30).Padding(5, 10).Gap(10).Radius(6).
		Justify(ui.Start).AlignItems(ui.Center).Label(it.Label)
	a.pickerRow(c, row, i == pk.sel, pk.moved)
	if row.Clicked() {
		a.post(func() { a.closePluginPicker(m.Index) })
	}
	row.Children(func() {
		if it.icon != nil {
			ui.Icon(c, it.icon).FontSize(14).TextColor(p.muted).Shrink(0)
		}
		ui.Column(c).Gap(1).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Baseline).Children(func() {
				ui.RichText(c, a.matchSpans(it.Label, 0, m.Positions, p.text)...).FontSize(13).SingleLine().Shrink(0)
				if it.Description != "" {
					ui.Text(c, it.Description).FontSize(12).TextColor(p.faint).SingleLine().Shrink(1).MinWidth(0)
				}
			})
			if it.Detail != "" {
				ui.Text(c, it.Detail).FontSize(11.5).TextColor(p.muted).SingleLine()
			}
		})
	})
}

// namedIcon is the icon a plugin names, of FiveLeaf's own: a plug for a name
// it does not know.
func namedIcon(name string) *ui.SVG {
	switch strings.ToLower(name) {
	case "check", "done":
		return iconCheck
	case "x", "close":
		return iconX
	case "plus", "add":
		return iconPlus
	case "minus":
		return iconMinus
	case "file":
		return iconFile
	case "code", "file-code":
		return iconFileCode
	case "text", "file-text":
		return iconFileText
	case "folder":
		return iconFolder
	case "search":
		return iconSearch
	case "star":
		return iconStar
	case "info":
		return iconInfo
	case "warning", "alert":
		return iconTriangleAlert
	case "error":
		return iconOctagonAlert
	case "trash", "delete":
		return iconTrash
	case "refresh", "reload":
		return iconRefresh
	case "settings", "gear":
		return iconSettings
	case "terminal":
		return iconTerminal
	case "branch", "git":
		return iconBranch
	case "commit":
		return iconCommit
	case "tag":
		return iconTag
	case "clock", "time":
		return iconClock
	case "lightbulb", "idea":
		return iconLightbulb
	case "sparkles", "ai":
		return iconSparkles
	case "comment", "message":
		return iconComment
	case "copy":
		return iconCopy
	case "external", "link":
		return iconExternal
	case "pencil", "edit":
		return iconPencil
	case "list", "tree":
		return iconListTree
	case "function":
		return iconFunction
	case "hash":
		return iconHash
	case "lock":
		return iconLock
	case "server":
		return iconServer
	case "cloud":
		return iconCloud
	case "send":
		return iconSend
	case "keyboard":
		return iconKeyboard
	case "box", "package":
		return iconBox
	case "braces", "json":
		return iconBraces
	case "download":
		return iconDownload
	case "image":
		return iconImage
	}
	return iconPlug
}
