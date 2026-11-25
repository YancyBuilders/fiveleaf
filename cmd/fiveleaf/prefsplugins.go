package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/smallnest/uix/components/button"

	"fiveleaf/internal/keymap"
	"fiveleaf/internal/plugin"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

// pluginsPage is what the Plugins page of the settings window keeps: what
// is typed to install, the index as last read, the plugins as last
// listed, and the work going on.
type pluginsPage struct {
	source string
	// busy says what is being done, "" for nothing, and target is the
	// plugin it is done to, whose row shows it: "" for one typed in.
	busy, target string
	// err is what the last install, update or removal failed with, which
	// the row of errFor shows, or the install card for "".
	err, errFor string
	// confirm is the plugin whose removal waits for a second click.
	confirm string

	index             []indexItem
	categories        []int
	indexOK           bool
	reading           bool
	indexErr          string
	installOpen       bool
	searchH           ui.Handle
	filters, shownFor pluginFilters
	shown             []pluginCatalogRow

	items  []pluginItem
	gen    uint64
	listed bool

	// detail is the plugin whose details show in place of the list, ""
	// for the list, and info what they show of it. back is set as the
	// list comes back from them, which slides it in from where they were;
	// listScroll keeps where the list was scrolled to meanwhile.
	detail     string
	info       pluginInfo
	back       bool
	listScroll ui.ScrollState
}

// pluginItem is an installed plugin as its row shows it: what is told of
// it beside its manifest, worked out as the plugins change, not as each
// frame draws.
type pluginItem struct {
	*plugin.Plugin
	initial, source, hay string
	canUpdate, linked    bool
}

// indexItem is an entry of the index as its row shows it, and what the
// filter looks in, lowercased once.
type indexItem struct {
	plugin.IndexEntry
	title, initial, source, hay string
	kinds                       []string
}

// pluginItems are the plugins installed, listed anew once they changed.
func (w *prefsWindow) pluginItems() []pluginItem {
	pg := &w.plugins
	plugins.mu.Lock()
	gen := plugins.gen
	plugins.mu.Unlock()
	if pg.listed && pg.gen == gen {
		return pg.items
	}
	list := plugins.reg.Plugins()
	pg.items = make([]pluginItem, len(list))
	for i, pl := range list {
		o := plugins.reg.Origin(pl.Name)
		it := pluginItem{Plugin: pl, initial: initial(pl.Title()), canUpdate: o.FromGit && !o.Source.Pinned(), linked: o.Linked}
		switch {
		case pl.Bundled:
			it.source = locales.PluginsBundled()
		case pl.Script:
			it.source = locales.PluginScripts()
		case o.FromGit:
			it.source = repoName(o.URL)
		case o.Linked:
			it.source = locales.PluginsDev()
		default:
			it.source = locales.PluginsLocal()
		}
		it.hay = strings.ToLower(strings.Join([]string{pl.Name, pl.DisplayName, pl.Description, pl.Category, pl.Author, it.source}, " "))
		for _, title := range pl.DisplayNames {
			it.hay += " " + strings.ToLower(title)
		}
		pg.items[i] = it
	}
	pg.shown = nil
	pg.gen, pg.listed = gen, true
	return pg.items
}

// initial is the first letter of a name, as its tile shows it.
func initial(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// repoName is a Git URL as short as it reads: owner/repo of GitHub.
func repoName(url string) string {
	url = strings.TrimSuffix(strings.TrimPrefix(url, "https://"), ".git")
	return strings.TrimPrefix(url, "github.com/")
}

// pluginRowMotion is how a row of a plugin comes and goes, as one is
// installed or removed: it opens and closes rather than jumps.
var pluginRowMotion = ui.ElementTransition{Duration: motionShow, Colors: true,
	Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}}

// pluginsPane is the Plugins page: the plugins installed, bundled and
// scripts, each with its switch and the way to its details; the
// shortcuts plugins could not take; and installing from the index or Git.
func (w *prefsWindow) pluginsPane(c *ui.Context) {
	p := w.pal
	pg := &w.plugins
	if pg.detail != "" && plugins != nil {
		items := w.pluginItems()
		for i := range items {
			if items[i].Name == pg.detail {
				w.pluginDetail(c, &items[i])
				return
			}
		}
		// Removed while its details showed.
		w.closeDetail()
	}
	w.responsiveHead(c, "prefs-head", iconPlug, prefSection(settings.CatPlugins), 310, func() {
		w.pluginCatalogActions(c)
	})
	if plugins != nil {
		w.pluginItems()
		w.pluginCatalogToolbar(c)
	}
	ui.Scroll(c.Key("prefs-rows-plugins")).Grow(1).MinHeight(0).TrackScroll(&pg.listScroll).Children(func() {
		page := ui.Column(c.Key("prefs-page-plugins")).Padding(20, 28, 32, 28).Gap(10).AlignSelf(ui.Center).
			WidthPercent(100).MaxWidth(prefsPageWidth)
		if pg.back {
			slideIn(c, page, -pageSlide)
		} else {
			fadePage(page)
		}
		page.Children(func() {
			if plugins == nil {
				w.emptyState(c, "plugins-none", iconPlug, locales.PluginsNotStarted())
			} else {
				if plugins.reg.Safe() {
					w.note(c, "plugins-safe", iconTriangleAlert, p.modifiedFg, locales.PluginsSafeMode(), nil)
				}
				if pg.installOpen {
					w.installCard(c)
				}
				w.errNote(c, devPluginTarget, 0)
				w.pluginCatalogResults(c)
				if skipped := w.skippedPluginKeys(); len(skipped) > 0 {
					w.sectionLabel(c, "plugins-keys-head", locales.PluginsSkippedKeys(), 0)
					w.card(c, "plugins-keys", func() {
						for i, k := range skipped {
							w.skippedKeyRow(c, i, k)
						}
					})
				}
			}
			w.sectionLabel(c, "plugins-settings-head", locales.PluginsFiveLeafSettings(), 0)
			w.card(c, "prefs-card-plugins", func() {
				first := true
				for _, f := range settings.Schema {
					if f.Category != settings.CatPlugins || f.Key == "plugins" || f.Memory {
						continue
					}
					if !first {
						ui.Row(c.Key("prefs-rule-"+f.Key)).Height(1).Margin(0, 16).Background(p.border).Shrink(0)
					}
					first = false
					w.row(c, f, nil)
				}
			})
		})
	})
}

// sectionLabel is the label above a card of the Plugins page, with how
// many the card lists when n is more than 0.
func (w *prefsWindow) sectionLabel(c *ui.Context, key, text string, n int) {
	p := w.pal
	ui.Row(c.Key(key)).Padding(10, 4, 0, 4).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, strings.ToUpper(text)).FontSize(10.5).FontWeight(600).LetterSpacing(0.7).TextColor(p.faint).SingleLine()
		if n > 0 {
			ui.Text(c.Key("count"), strconv.Itoa(n)).FontSize(10).FontWeight(600).TextColor(p.muted).Padding(0, 6).Radius(8).
				MinWidth(18).TextAlign(ui.Center).Background(p.text.Alpha(0.07)).ContentTransition(ui.ContentTransition{Numeric: true})
		}
	})
}

// emptyState says a card has nothing to list, under a badge of icon.
func (w *prefsWindow) emptyState(c *ui.Context, key string, icon *ui.SVG, text string) {
	p := w.pal
	ui.Column(c.Key(key)).Padding(28, 24).Gap(10).AlignItems(ui.Center).Children(func() {
		popUp(ui.Box(c.Key("badge")).Size(40, 40).Radius(20).Background(p.text.Alpha(0.06)).Justify(ui.Center).AlignItems(ui.Center)).Children(func() {
			ui.Icon(c, icon).FontSize(17).TextColor(p.muted)
		})
		ui.Text(c, text).FontSize(12.5).LineHeight(1.4).TextColor(p.muted).TextAlign(ui.Center).MaxWidth(420)
	})
}

// note is a line that stands out of the page, in col, as a warning or an
// error does: the text can be chosen to copy, and dismiss, when set, is
// what its close button does.
func (w *prefsWindow) note(c *ui.Context, key string, icon *ui.SVG, col ui.Color, text string, dismiss func()) ui.Element {
	p := w.pal
	return ui.Row(c.Key(key)).Padding(8, 8, 8, 12).Gap(9).Radius(8).AlignItems(ui.Start).Background(col.Alpha(0.08)).
		Border(1, col.Alpha(0.28)).Transition(popIn).Children(func() {
		ui.Icon(c, icon).FontSize(13).TextColor(col).Shrink(0).Margin(2, 0, 0, 0)
		ui.Text(c, text).FontSize(12).LineHeight(1.4).TextColor(p.text).Grow(1).Shrink(1).MinWidth(0).Selectable().Margin(1, 0, 0, 0)
		if dismiss == nil {
			return
		}
		x := ui.ButtonBase(c.Key("dismiss")).Size(20, 20).Radius(5).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
			Label(locales.PrefsDismiss()).Tooltip(locales.PrefsDismiss())
		if x.Hovered() {
			x.Background(col.Alpha(0.14))
		}
		x.Transition(hoverFade)
		pressPose(x)
		x.Children(func() { ui.Icon(c, iconX).FontSize(10.5).TextColor(p.muted) })
		if x.Clicked() {
			dismiss()
		}
	})
}

// errNote shows what the work on the plugin of target failed with, if it
// did, with the button that dismisses it, indent in from the start.
func (w *prefsWindow) errNote(c *ui.Context, target string, indent float32) {
	pg := &w.plugins
	if pg.err == "" || pg.errFor != target {
		return
	}
	w.note(c, "plugins-err", iconOctagonAlert, w.pal.dangerFg, pg.err, func() { pg.err, pg.errFor = "", "" }).Margin(0, 0, 0, indent)
}

// busyLine shows what is being done to the plugin of target, if it is,
// indent in from the start.
func (w *prefsWindow) busyLine(c *ui.Context, target string, indent float32) {
	pg := &w.plugins
	if pg.busy == "" || pg.target != target {
		return
	}
	ui.Row(c.Key("plugins-busy")).Gap(8).AlignItems(ui.Center).Margin(0, 0, 0, indent).Transition(revealFade).Children(func() {
		ui.Spinner(c).Size(11, 11)
		ui.Text(c, pg.busy).FontSize(12).TextColor(w.pal.muted).SingleLine().Shrink(1)
	})
}

// tileIndent is how far the text of a row of a plugin starts from its
// tile's start, where what is under the text lines up.
const tileIndent = 34 + 12

// tile is the square of a plugin's initial at the start of its row, in
// the accent while it is on.
func (w *prefsWindow) tile(c *ui.Context, letter string, icon *ui.SVG, on bool) {
	w.tileOf(c, letter, icon, on, 34)
}

// tileOf is a plugin's tile of side size, its corners and letter in
// proportion.
func (w *prefsWindow) tileOf(c *ui.Context, letter string, icon *ui.SVG, on bool, size float32) {
	p := w.pal
	bg, fg := p.accent.Alpha(0.13), p.accent
	if !on {
		bg, fg = p.text.Alpha(0.06), p.muted
	}
	glyph := size * 0.44
	ui.Box(c.Key("tile")).Size(size, size).Radius(size * 0.265).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Background(bg).
		Transition(hoverFade).Children(func() {
		if icon != nil {
			ui.Icon(c.Key("glyph"), icon).FontSize(glyph).TextColor(fg).Transition(hoverFade)
		} else {
			ui.Text(c.Key("glyph"), letter).FontSize(glyph).FontWeight(600).TextColor(fg).Transition(hoverFade)
		}
	})
}

// pill is a short word beside a plugin's name, as its version, in col.
func (w *prefsWindow) pill(c *ui.Context, key, text string, col ui.Color, mono bool) {
	t := ui.Text(c.Key(key), text).FontSize(10.5).TextColor(col).Padding(0, 6).Radius(5).Background(col.Alpha(0.12)).
		SingleLine().Shrink(0)
	if mono {
		t.Font(w.monoFont())
	} else {
		t.FontWeight(600)
	}
}

// rowHover gives a row of a card the pointer's tint, and the rule above
// it, inset, unless it is the first.
func (w *prefsWindow) rowHover(row ui.Element, rule bool) {
	p := w.pal
	if row.Hovered() {
		row.Background(p.text.Alpha(0.035))
	}
	if rule {
		border := p.border
		row.Draw(func(pt *ui.Painter, r ui.Rect) {
			pt.Fill(ui.Rect{X: r.X + 16, Y: r.Y, W: r.W - 32, H: 1}, border, 0)
		})
	}
}

// iconButton is a small button of a row that shows only its icon, which
// tints with col under the pointer.
func (w *prefsWindow) iconButton(c *ui.Context, key string, icon *ui.SVG, label string, col ui.Color, disabled bool) bool {
	p := w.pal
	b := ui.ButtonBase(c.Key(key)).Size(26, 26).Radius(6).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
		Label(label).Tooltip(label).Disabled(disabled)
	fg := p.muted
	switch {
	case disabled:
		fg = p.faint.Alpha(0.5)
	case b.Hovered():
		b.Background(col.Alpha(0.12))
		fg = col
	}
	b.Transition(hoverFade)
	pressPose(b)
	b.Children(func() { ui.Icon(c.Key("icon"), icon).FontSize(13).TextColor(fg).Transition(hoverFade) })
	return b.Clicked() && !disabled
}

// primaryButton is a button in the accent, of what a card is for, as
// Install: disabled, it lays its accent aside for a quiet face, rather
// than show it dimmed, which reads as all but enabled on a dark window.
func (w *prefsWindow) primaryButton(c *ui.Context, key, label string, disabled bool) ui.Element {
	p := w.pal
	b := compactActionButton(c.Key(key), label, nil, button.Primary).Disabled(disabled)
	if disabled {
		b.Background(p.text.Alpha(0.06)).TextColor(p.faint).Border(1, p.border)
	}
	return b
}

// dangerButton is a button that loses something, as a removal's confirm.
func (w *prefsWindow) dangerButton(c *ui.Context, key, label string) bool {
	p := w.pal
	b := compactActionButton(c.Key(key), label, nil, button.Primary)
	bg := p.dangerFill
	switch {
	case b.Pressed():
		bg = p.dangerPressed
	case b.Hovered():
		bg = p.dangerHover
	}
	b.Background(bg).TextColor(p.dangerText)
	return b.Clicked()
}

// pluginState is what a plugin's row says of how it is doing, and the
// color of the dot before it.
func (w *prefsWindow) pluginState(st plugin.Stats) (string, ui.Color) {
	return pluginStateOf(w.pal, st)
}

// pluginStateOf is how a plugin is doing, in words, and the color of its
// dot.
func pluginStateOf(p *palette, st plugin.Stats) (string, ui.Color) {
	switch st.State {
	case plugin.StateOff:
		return locales.PluginsOff(), p.faint
	case plugin.StateCrashed:
		return locales.PluginsStopped(), p.dangerFg
	case plugin.StateRunning:
		return locales.PluginsRunning(st.ActivatedIn.Round(time.Millisecond).String()), p.addedFg
	case plugin.StateStatic:
		return locales.PluginsDeclares(), p.accent
	}
	// Starting, or waiting for its first call: in trouble when it has
	// failed, or cannot be read.
	if st.Err != "" {
		return locales.PluginsIdle(), p.dangerFg
	}
	return locales.PluginsIdle(), p.faint
}

func (w *prefsWindow) pluginToggle(c *ui.Context, pl *plugin.Plugin, on bool) ui.Element {
	switchControl := w.toggle(c, &on).Label(locales.PluginsEnable(pl.Title())).Disabled(pl.MissingAgentCommand != "")
	if switchControl.Changed() {
		plugins.setEnabled(pl.Name, on)
	}
	return switchControl
}

func (w *prefsWindow) agentCommandWarning(c *ui.Context, pl *plugin.Plugin, indent float32) {
	if pl.MissingAgentCommand != "" {
		w.note(c, "agent-command-warning", iconTriangleAlert, w.pal.modifiedFg, locales.PluginsMissingAgentCommand(pl.MissingAgentCommand), nil).Margin(0, 0, 0, indent)
	}
}

// pluginRow is a plugin of the list: its tile, name, version and what it
// is; how it is doing and where it came from; its buttons and switch; and
// the way to its settings. rule draws the line above it.
func (w *prefsWindow) pluginRow(c *ui.Context, it *pluginItem, indexed *indexItem, rule bool) {
	p := w.pal
	pg := &w.plugins
	pl := it.Plugin
	st := pl.Stats()
	on := !st.Disabled
	busy := pg.busy != ""
	row := ui.Column(c.Key("plugin-row-"+pl.Name)).Padding(14, 16).Gap(10).Transition(pluginRowMotion)
	w.rowHover(row, rule)
	row.Children(func() {
		// The head of the row opens the plugin's details; its buttons and
		// switch take their own clicks.
		head := ui.Row(c.Key("head")).Gap(12).AlignItems(ui.Start).Focusable().Label(locales.PluginsDetails(pl.Title()))
		if head.Clicked() {
			w.openDetail(pl.Name)
		}
		hovered := row.Hovered()
		head.Children(func() {
			var glyph *ui.SVG
			if pl.Script {
				glyph = iconFileCode
			}
			w.tile(c, it.initial, glyph, on)
			ui.Column(c).Gap(3).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					title := p.text
					if !on {
						title = p.muted
					}
					ui.Text(c.Key("title"), pl.Title()).FontSize(13).FontWeight(600).TextColor(title).Shrink(1).SingleLine().
						Transition(hoverFade)
					if pl.Version != "" {
						w.pill(c, "version", pl.Version, p.muted, true)
					}
					if it.canUpdate && plugins.updateWaits(pl.Name) {
						w.pill(c, "update-waits", locales.PluginsUpdateWaits(), p.accent, false)
					}
				})
				if pl.Description != "" {
					ui.Text(c, pl.Description).FontSize(12).LineHeight(1.4).TextColor(p.muted)
				}
				state, dot := w.pluginState(st)
				ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(1, 0, 0, 0).Children(func() {
					ui.Box(c.Key("dot")).Size(6, 6).Radius(3).Shrink(0).Background(dot).Transition(hoverFade)
					ui.Text(c, state).FontSize(11).TextColor(p.muted).SingleLine().Shrink(0)
					ui.Text(c, "·").FontSize(11).TextColor(p.faint)
					ui.Text(c, pl.Name).Font(w.monoFont()).FontSize(10.5).TextColor(p.faint).SingleLine().Shrink(0)
					if label := pluginCategoryLabel(pl.Category); label != "" {
						ui.Text(c, "·").FontSize(11).TextColor(p.faint)
						ui.Text(c, label).FontSize(11).TextColor(p.faint).SingleLine().Shrink(0)
					}
					ui.Text(c, "·").FontSize(11).TextColor(p.faint)
					ui.Text(c, it.source).FontSize(11).TextColor(p.faint).SingleLine().Shrink(1).MinWidth(0)
					if pl.Author != "" {
						ui.Text(c, "·").FontSize(11).TextColor(p.faint)
						ui.Text(c, locales.PluginsBy(pl.Author)).FontSize(11).TextColor(p.faint).SingleLine().Shrink(1).MinWidth(0)
					}
				})
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).MinHeight(34).Children(func() {
				w.installToolboxRelease(c, it, indexed)
				if it.canUpdate && !pl.Bundled {
					if actionButton(c.Key("update"), locales.PluginsUpdate(), nil).Disabled(busy).Clicked() {
						w.updatePlugin(pl.Name)
					}
				}
				if !pl.Bundled && !pl.Script {
					if w.iconButton(c, "remove", iconTrash, locales.PluginsRemove(), p.dangerFg, busy) {
						pg.confirm = pl.Name
					}
				}
				w.pluginToggle(c.Key("on"), pl, on)
				// The way into the details, which leans toward them under
				// the pointer.
				col, pose := p.faint, ui.Pose{}
				if hovered {
					col, pose.X = p.muted, 2
				}
				ui.Icon(c.Key("more"), iconChevronRight).FontSize(13).TextColor(col).Margin(0, 0, 0, 2).Transition(hoverFade).
					AnimatePose(pose, ui.Snappy)
			})
		})
		w.agentCommandWarning(c, pl, tileIndent)
		if st.Err != "" {
			w.note(c, "plugin-err", iconOctagonAlert, p.dangerFg, firstLine(st.Err), nil).Margin(0, 0, 0, tileIndent)
		}
		w.busyLine(c, pl.Name, tileIndent)
		w.errNote(c, pl.Name, tileIndent)
		if pg.confirm == pl.Name {
			w.confirmRemove(c, it, tileIndent)
		}
		if n := len(pl.Contributes.Settings); n > 0 {
			w.settingsLink(c, pl, n)
		}
	})
}

func (w *prefsWindow) installToolboxRelease(c *ui.Context, installed *pluginItem, indexed *indexItem) {
	if indexed.replacesStock(installed) {
		release, _ := indexed.Newest()
		if actionButton(c.Key("install-marketplace"), locales.PluginsInstall()+" "+release.Version, iconDownload).
			Disabled(w.plugins.busy != "").Clicked() {
			w.installPlugin(indexed.Name, &indexed.IndexEntry)
		}
	}
}

// settingsLink says how many settings a plugin has, under its row, and
// opens its details, where they are set.
func (w *prefsWindow) settingsLink(c *ui.Context, pl *plugin.Plugin, n int) {
	p := w.pal
	b := ui.ButtonBase(c.Key("settings-link")).Margin(0, 0, 0, tileIndent-6).Padding(3, 6).Gap(6).Radius(6).AlignSelf(ui.Start).
		AlignItems(ui.Center).Label(locales.PluginsDetails(pl.Title()))
	col, pose := p.muted, ui.Pose{}
	if b.Hovered() {
		b.Background(p.hover)
		col, pose.X = p.text, 2
	}
	b.Transition(hoverFade)
	pressPose(b)
	b.Children(func() {
		ui.Icon(c, iconSettings).FontSize(12).TextColor(p.faint)
		ui.Text(c.Key("label"), locales.PluginsSettingsCount(n)).FontSize(11.5).TextColor(col).SingleLine().Transition(hoverFade)
		ui.Icon(c.Key("go"), iconChevronRight).FontSize(10.5).TextColor(p.faint).AnimatePose(pose, ui.Snappy)
	})
	if b.Clicked() {
		w.openDetail(pl.Name)
	}
}

// confirmRemove asks to remove a plugin, as its button was clicked,
// indent in from the start.
func (w *prefsWindow) confirmRemove(c *ui.Context, pl *pluginItem, indent float32) {
	p := w.pal
	pg := &w.plugins
	ui.Row(c.Key("confirm-remove")).Padding(8, 8, 8, 12).Margin(0, 0, 0, indent).Gap(8).Radius(8).AlignItems(ui.Center).
		Background(p.dangerFg.Alpha(0.08)).Border(1, p.dangerFg.Alpha(0.28)).Transition(popIn).Children(func() {
		ui.Icon(c, iconTrash).FontSize(13).TextColor(p.dangerFg).Shrink(0)
		question := locales.PluginsRemoveAsk(pl.Title())
		if pl.linked {
			question = locales.PluginsUnlinkAsk(pl.Title())
		}
		ui.Text(c, question).FontSize(12).TextColor(p.text).Grow(1).Shrink(1).MinWidth(0)
		if actionButton(c.Key("cancel"), locales.ButtonCancel(), nil).Clicked() {
			pg.confirm = ""
		}
		if w.dangerButton(c, "remove-yes", locales.PluginsRemove()) {
			pg.confirm = ""
			w.removePlugin(pl.Name)
		}
	})
}

// removePlugin removes a plugin's folder in the background.
func (w *prefsWindow) removePlugin(name string) {
	pg := &w.plugins
	pg.busy, pg.target, pg.err, pg.errFor = locales.PluginsRemoving(name), name, "", ""
	reg := plugins.reg
	go func() {
		err := plugin.Remove(reg.Dirs.Plugins, name)
		if err == nil {
			reg.Load()
		}
		w.post(func() {
			pg.busy, pg.target = "", ""
			if err != nil {
				pg.err, pg.errFor = err.Error(), name
				return
			}
			w.notes = append(w.notes, ui.Toast{ID: "removed-" + name, Title: locales.PluginsRemoved(name)})
		})
	}()
}

// installCard installs a plugin from a Git URL, owner/repo, or a name of
// the index.
func (w *prefsWindow) installCard(c *ui.Context) {
	p := w.pal
	pg := &w.plugins
	w.card(c, "plugins-install", func() {
		ui.Column(c).Padding(14, 16).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconDownload).FontSize(13).TextColor(p.muted).Shrink(0)
				ui.Text(c, locales.PluginsInstallTitle()).FontSize(13).FontWeight(500).TextColor(p.text)
				ui.Spacer(c)
				if w.iconButton(c, "dismiss", iconX, locales.PrefsDismiss(), p.muted, pg.busy != "") {
					pg.installOpen = false
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ready := strings.TrimSpace(pg.source) != "" && pg.busy == ""
				in := ui.TextInput(c.Key("plugins-source"), &pg.source).AutoFocus().Grow(1).MinWidth(0).FontSize(12.5).
					Placeholder(locales.PluginsInstallPlaceholder()).Label(locales.PluginsInstallTitle())
				go_ := w.primaryButton(c, "plugins-install-go", locales.PluginsInstall(), !ready)
				pressPose(go_)
				if (go_.Clicked() || in.Submitted()) && ready {
					w.installPlugin(strings.TrimSpace(pg.source), nil)
				}
			})
			w.busyLine(c, "", 0)
			w.errNote(c, "", 0)
		})
	})
}

const devPluginTarget = ":development"

func (w *prefsWindow) chooseDevPlugin() {
	pg := &w.plugins
	if plugins == nil || pg.busy != "" {
		return
	}
	parent, reg := w.win, plugins.reg
	title := locales.PluginsInstallDev()
	pg.busy, pg.target, pg.err, pg.errFor = title, devPluginTarget, "", ""
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: title, ButtonLabel: locales.PluginsInstall(), Directory: true})
		var manifest *plugin.Manifest
		if err == nil && len(paths) > 0 {
			manifest, err = reg.Dirs.LinkDev(paths[0])
			if err == nil {
				reg.Load()
			}
		}
		w.post(func() {
			pg.busy, pg.target = "", ""
			if err != nil {
				pg.err, pg.errFor, pg.listScroll.Y = locales.FailWithError(title, err.Error()), devPluginTarget, 0
			} else if manifest != nil {
				w.notes = append(w.notes, ui.Toast{ID: "linked-" + manifest.Name, Title: locales.PluginsInstalled(manifest.Name)})
				w.openDetail(manifest.Name)
			}
		})
	}()
}

// installPlugin installs a plugin in the background, as the command does;
// entry is the selected index row, or nil for a typed source.
func (w *prefsWindow) installPlugin(spec string, entry *plugin.IndexEntry) {
	pg := &w.plugins
	target := ""
	ref := ""
	if entry != nil {
		target = entry.Name
	} else if name, selectedRef, byName := plugin.SplitSpec(spec); byName && pg.index != nil {
		match := slices.IndexFunc(pg.index, func(item indexItem) bool { return item.Name == name })
		if match < 0 {
			pg.err, pg.errFor = pg.indexErr, ""
			if pg.err == "" {
				pg.err = locales.PluginsNotInIndex(name)
			}
			return
		}
		if match >= 0 {
			entry, ref = &pg.index[match].IndexEntry, selectedRef
		}
	}
	pg.busy, pg.target, pg.err, pg.errFor = locales.PluginsInstalling(spec), target, "", ""
	host := plugins
	reg := host.reg
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		var m *plugin.Manifest
		var err error
		if entry == nil {
			m, err = plugin.InstallSpec(ctx, reg.Dirs.Plugins, spec, nil)
		} else {
			m, err = entry.Install(ctx, reg.Dirs.Plugins, ref)
		}
		if err == nil {
			host.updated(m.Name)
			reg.Load()
		}
		w.post(func() {
			pg.busy, pg.target = "", ""
			if err != nil {
				pg.err, pg.errFor = err.Error(), target
				if name, ok := err.(plugin.IndexNameNotFound); ok {
					pg.err = locales.PluginsNotInIndex(string(name))
				}
				return
			}
			if target == "" {
				pg.source, pg.installOpen = "", false
			}
			w.notes = append(w.notes, ui.Toast{ID: "installed-" + m.Name, Title: locales.PluginsInstalled(m.Name)})
		})
	}()
}

// post runs fn on the main thread and draws the window.
func (w *prefsWindow) post(fn func()) {
	mygo.RunOnMain(func() {
		fn()
		w.invalidate()
	})
}

// updatePlugin updates a plugin installed from Git in the background.
func (w *prefsWindow) updatePlugin(name string) {
	pg := &w.plugins
	pg.busy, pg.target, pg.err, pg.errFor = locales.PluginsUpdating(name), name, "", ""
	reg := plugins.reg
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		changed, err := plugin.Update(ctx, reg.Dirs.Plugins, name)
		w.post(func() {
			pg.busy, pg.target = "", ""
			switch {
			case err != nil:
				pg.err, pg.errFor = err.Error(), name
			case changed:
				plugins.updated(name)
				w.notes = append(w.notes, ui.Toast{ID: "updated-" + name, Title: locales.PluginsUpdated(name)})
				go reg.Load()
			default:
				w.notes = append(w.notes, ui.Toast{ID: "uptodate-" + name, Title: locales.PluginsUpToDate(name)})
			}
		})
	}()
}

// pluginSettingControl edits a setting of a plugin by its type.
func (w *prefsWindow) pluginSettingControl(c *ui.Context, name, key string, def plugin.SettingContrib, raw json.RawMessage) {
	k := c.Key("control")
	switch {
	case def.Type == "boolean":
		var on bool
		json.Unmarshal(raw, &on)
		if w.toggle(k, &on).Label(key).Changed() {
			w.setPluginSetting(name, key, on, nil)
		}
	case len(def.Enum) > 0:
		var v string
		json.Unmarshal(raw, &v)
		labels := def.Enum
		selected := slices.Index(labels, v)
		optional := len(def.Default) == 0
		if optional {
			labels = append([]string{locales.KeysDefault()}, labels...)
			selected++
		}
		selected = max(selected, 0)
		var changed bool
		if segmentedFits(labels) {
			selected, changed = w.segmented(c, "seg-"+name+"-"+key, selected, labels, key, false)
		} else {
			choice := labels[selected]
			changed = ui.Select(k, &choice, labels).Label(key).Width(200).Changed()
			selected = slices.Index(labels, choice)
		}
		if changed && selected >= 0 {
			if optional && selected == 0 {
				w.setPluginSetting(name, key, nil, nil)
			} else {
				w.setPluginSetting(name, key, labels[selected], nil)
			}
		}
	default:
		id := "plugin:" + name + "." + key
		var s string
		if json.Unmarshal(raw, &s) != nil {
			s = strings.TrimSpace(string(raw))
		}
		buf := w.textBuffer(id, s)
		in := ui.TextInput(k, buf).Width(200).FontSize(12.5).Label(key)
		if len(def.Default) == 0 {
			in.Placeholder(locales.KeysDefault())
		}
		w.draft(id, s, buf, in.Focused(), in.Submitted(), func(v any) {
			text, _ := v.(string)
			var out any = text
			if def.Type == "number" || def.Type == "integer" {
				text = strings.TrimSpace(text)
				if text == "" {
					w.setPluginSetting(name, key, nil, nil)
					return
				}
				value, err := def.ArgumentValue(json.RawMessage(text))
				if err != nil {
					w.errors = append(w.errors, fmt.Sprintf("%s: %v", key, err))
					*buf = s
					return
				}
				out = json.Number(value)
			}
			w.setPluginSetting(name, key, out, nil)
		})
	}
}

// setPluginSetting sets plugins.<name>.<key> in settings.json, or takes it
// away when v is deflt, as "enabled" is when it matches the plugin's default.
func (w *prefsWindow) setPluginSetting(name, key string, v, deflt any) {
	w.set("plugins", withPluginSetting(w.m.s.Prefs.Plugins, name, key, v, deflt))
	if plugins != nil {
		plugins.syncPrefs()
		plugins.Changed()
		plugins.reg.Emit("settings.changed", map[string]any{})
	}
}

// withPluginSetting is the plugins' settings all, with the setting of key
// of the plugin of name v, or left out as deflt.
func withPluginSetting(all map[string]json.RawMessage, name, key string, v, deflt any) json.RawMessage {
	all = maps.Clone(all)
	if all == nil {
		all = map[string]json.RawMessage{}
	}
	var obj map[string]json.RawMessage
	json.Unmarshal(all[name], &obj)
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	if v == nil || deflt != nil && v == deflt {
		delete(obj, key)
	} else {
		data, _ := json.Marshal(v)
		obj[key] = data
	}
	if len(obj) == 0 {
		delete(all, name)
	} else {
		data, _ := json.Marshal(obj)
		all[name] = data
	}
	data, _ := json.Marshal(all)
	return data
}

// skippedPluginKeys are the shortcuts of plugins that keys FiveLeaf or the
// user bind took.
func (w *prefsWindow) skippedPluginKeys() []plugin.KeyContrib {
	if len(w.m.wins) == 0 {
		return nil
	}
	return w.m.wins[0].a.skippedKeys()
}

// skippedKeyRow offers a shortcut a plugin could not take, which a click
// writes to keybindings.json, where it wins.
func (w *prefsWindow) skippedKeyRow(c *ui.Context, i int, k plugin.KeyContrib) {
	p := w.pal
	row := ui.Row(c.Key(fmt.Sprintf("plugins-key-%d", i))).Padding(10, 16).Gap(10).AlignItems(ui.Center)
	w.rowHover(row, i > 0)
	row.Children(func() {
		ui.Text(c, k.Command).Font(w.monoFont()).FontSize(11.5).TextColor(p.text).Grow(1).Shrink(1).SingleLine()
		ui.Text(c, k.Key).Font(w.monoFont()).FontSize(11.5).TextColor(p.muted)
		if actionButton(c.Key("use"), locales.PluginsUseShortcut(), nil).Clicked() {
			path := keymap.Path(w.m.s.Path())
			data, _ := os.ReadFile(path)
			if len(data) == 0 {
				data = []byte("[]\n")
			}
			out, err := keymap.Edit(data, nil, []keymap.Entry{{Key: k.Key, Command: k.Command, When: k.When}})
			if err == nil {
				err = os.WriteFile(path, out, 0o644)
			}
			if err != nil {
				w.errors = append(w.errors, err.Error())
			}
		}
	})
}

// readIndex reads the index in the background.
func (w *prefsWindow) readIndex() {
	pg := &w.plugins
	pg.reading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, err := plugin.FetchIndex(ctx, nil)
		items := indexItems(entries)
		w.post(func() {
			pg.finishIndexRead(items, err)
		})
	}()
}

func (pg *pluginsPage) finishIndexRead(items []indexItem, err error) {
	pg.reading, pg.indexOK = false, true
	switch {
	case err == nil:
		pg.setIndex(items)
		pg.indexErr = ""
	case len(items) > 0:
		if pg.index == nil {
			pg.setIndex(items)
		}
		pg.indexErr = locales.PluginsIndexSaved(err.Error())
	default:
		pg.indexErr = locales.PluginsIndexFailed(err.Error())
	}
}

// indexItems are the entries of the index as their rows show them.
func indexItems(entries []plugin.IndexEntry) []indexItem {
	if entries == nil {
		return nil
	}
	items := make([]indexItem, len(entries))
	for i, e := range entries {
		title := e.DisplayName
		if title == "" {
			title = e.Name
		}
		source := repoName(e.Repository)
		items[i] = indexItem{IndexEntry: e, title: title, initial: initial(title), source: source,
			hay: strings.ToLower(strings.Join([]string{e.Name, title, e.Description, e.Category, e.Author, source, strings.Join(e.Tags, " ")}, " "))}
		for _, tag := range e.Tags {
			items[i].kinds = append(items[i].kinds, pluginTagKey.Replace(strings.ToLower(strings.TrimSpace(tag))))
		}
	}
	return items
}

// indexRow shows an available plugin and its install action.
func (w *prefsWindow) indexRow(c *ui.Context, e *indexItem, rule bool) {
	p := w.pal
	pg := &w.plugins
	row := ui.Column(c.Key("index-"+e.Name)).Padding(10, 16).Gap(8)
	w.rowHover(row, rule)
	row.Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			w.tile(c, e.initial, nil, false)
			ui.Column(c).Gap(2).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Text(c, e.title).FontSize(12.5).FontWeight(600).TextColor(p.text).SingleLine()
				if e.Description != "" {
					ui.Text(c, e.Description).FontSize(12).LineHeight(1.4).TextColor(p.muted)
				}
				ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center).Padding(1, 0, 0, 0).Children(func() {
					w.pill(c, "version", e.Version, p.muted, true)
					category := e.Category
					if label := pluginCategoryLabel(category); label != "" {
						w.pill(c, "category-"+category, label, p.muted, false)
					}
					ui.Text(c, e.source).FontSize(11).TextColor(p.faint).SingleLine().Shrink(1).MinWidth(0)
					if e.Author != "" {
						ui.Text(c, "·").FontSize(11).TextColor(p.faint)
						ui.Text(c, locales.PluginsBy(e.Author)).FontSize(11).TextColor(p.faint).SingleLine().Shrink(1).MinWidth(0)
					}
				})
			})
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
				if pg.busy != "" && pg.target == e.Name {
					ui.Spinner(c.Key("installing")).Size(13, 13).Margin(0, 8).Label(pg.busy)
				} else {
					b := actionButton(c.Key("install"), locales.PluginsInstall(), iconDownload).Disabled(pg.busy != "")
					pressPose(b)
					if b.Clicked() {
						w.installPlugin(e.Name, &e.IndexEntry)
					}
				}
			})
		})
		w.errNote(c, e.Name, tileIndent)
	})
}

// indexPlaceholders stand where the index's rows will, while it is read:
// a tile and the lines of a row, which glow and fade, or keep still with
// less motion.
func (w *prefsWindow) indexPlaceholders(c *ui.Context) {
	p := w.pal
	glow := float32(0.5)
	if !c.Preferences().ReduceMotion {
		t := float64(c.Now().UnixMilli()%1400) / 1400
		glow = float32(0.5 - 0.5*math.Cos(2*math.Pi*t))
		c.AnimationFrame()
	}
	col := p.text.Alpha(0.05 + 0.04*glow)
	ui.Column(c.Key("index-reading")).Label(locales.PluginsIndexReading()).Transition(revealFade).Children(func() {
		for i, width := range []float32{0.42, 0.56, 0.34} {
			ui.Box(c.Key(fmt.Sprintf("placeholder-%d", i))).Height(62).Draw(func(pt *ui.Painter, r ui.Rect) {
				if i > 0 {
					pt.Fill(ui.Rect{X: r.X + 16, Y: r.Y, W: r.W - 32, H: 1}, p.border, 0)
				}
				pt.Fill(ui.Rect{X: r.X + 16, Y: r.Y + 14, W: 34, H: 34}, col, 9)
				x, span := r.X+62, r.W-62-16
				pt.Fill(ui.Rect{X: x, Y: r.Y + 17, W: span * width, H: 10}, col, 5)
				pt.Fill(ui.Rect{X: x, Y: r.Y + 35, W: span * (width + 0.3), H: 8}, col, 4)
			})
		}
	})
}
