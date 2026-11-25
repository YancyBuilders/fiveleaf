package main

import (
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/keymap"
	"fiveleaf/locales"
)

// quickSettingsMenu keeps frequent settings and layout commands at hand.
func (a *app) quickSettingsMenu(m *ui.Menu) {
	commands := make(map[string]command)
	for _, cmd := range a.commands() {
		commands[cmd.id] = cmd
	}
	add := func(m *ui.Menu, id string) {
		cmd, ok := commands[id]
		if !ok {
			return
		}
		item := m.Item(cmd.label)
		if st, ok := keymap.ParseAccel(a.menuAccel(cmd)); ok {
			item.Shortcut(st.Mods, st.Key)
		}
		switch id {
		case "view.sidebar":
			item.Checked(!a.window.SidebarClosed)
		}
		if item.Chosen() {
			a.post(func() { a.run(id) })
		}
	}
	add(m, "go.commands")
	m.Separator()
	for _, id := range []string{"file.settingsJSON", "file.settings", "plugins.manage", "file.keybindings"} {
		add(m, id)
	}
	m.Submenu(locales.MenuTheme(), func(sub *ui.Menu) {
		add(sub, "view.selectTheme")
		add(sub, "view.selectIconTheme")
	})
	m.Submenu(locales.MenuView(), func(sub *ui.Menu) {
		for _, id := range []string{"view.sidebar", "view.terminal"} {
			add(sub, id)
		}
	})
	if update := a.host.update(); update.phase != updateDisabled {
		m.Separator()
		if m.Item(updateMenuLabel(update.version)).Chosen() {
			a.post(a.openUpdates)
		}
	}
}
