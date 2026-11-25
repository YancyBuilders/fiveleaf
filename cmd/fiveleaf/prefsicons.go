package main

import (
	"github.com/egoist/mygo/ui"

	"fiveleaf/locales"
)

func (w *prefsWindow) iconThemeTiles(c *ui.Context, chosen string, apply func(any)) {
	p := w.pal
	choices := allIconThemes()
	sel, available := 0, false
	for i, choice := range choices {
		if choice.matches(chosen) {
			sel, available = i, true
		}
	}
	was := sel
	clicked := -1
	tiles := ui.SegmentedBase(c.Key("icon-themes"), &sel, len(choices))
	tiles.Track.Wrap().Gap(8).Label(prefTitle("iconTheme")).Children(func() {
		for i, choice := range choices {
			on := available && i == sel
			tile := tiles.Segment(i).Row().Width(160).Height(34).Padding(8, 8).Gap(8).
				Radius(p.metricsOr().radius).AlignItems(ui.Center).Label(choice.name).
				Tooltip(choice.name).Border(1, p.border).FocusRing(false).Transition(hoverFade)
			if i == sel {
				w.first(tile)
			}
			if on {
				tile.Background(p.active).BorderColor(p.accent)
			} else if tile.Hovered() {
				tile.Background(p.hover)
			}
			if tile.FocusVisible() {
				tile.BorderColor(p.accent)
			}
			tile.Children(func() {
				ui.Row(c.Key("preview")).Gap(5).AlignItems(ui.Center).Children(func() {
					icon, tint := folderIconForTheme(p, choice.id, "src", true)
					iconThemeView(c.Key("folder"), p, icon, tint, 18)
					for _, name := range []string{"main.go", "app.tsx", "package.json", "README.md"} {
						icon, tint := fileIconForTheme(p, choice.id, name)
						iconThemeView(c.Key(name), p, icon, tint, 18)
					}
				})
				ui.Text(c.Key("name"), choice.name).FontSize(11.5).TextColor(p.text).SingleLine().MaxWidth(88)
			})
			if tile.Clicked() {
				clicked = i
			}
		}
	})
	// Clicking the fallback tile also clears an unavailable saved choice.
	if clicked >= 0 && (!available || choices[clicked].id != chosen) {
		apply(choices[clicked].id)
	} else if sel != was {
		apply(choices[sel].id)
	}
	if !available {
		ui.Text(c.Key("unavailable"), locales.PrefIconThemeUnavailable(chosen)).
			FontSize(12).TextColor(p.muted).Padding(4, 0)
	}
}
