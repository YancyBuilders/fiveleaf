package main

import "fiveleaf/internal/plugin"

type toolboxTab struct {
	provider *plugin.Plugin
	surface  toolboxSurface
}

func (a *app) openToolboxTab(name string) {
	if a.window.Detached {
		a.host.inWorkspace(func(workspace *app) { workspace.openToolboxTab(name) })
		return
	}
	if provider := plugins.toolboxProvider(name); provider != nil {
		a.showTool(&toolView{kind: toolToolbox, toolbox: &toolboxTab{provider: provider}})
	}
}

func (a *app) openToolboxPicker() {
	a.quickToolsPicker()
	a.invalidate()
}

func (a *app) focusToolboxTab(page *toolboxTab) {
	for index, col := range a.cols {
		if shown := col.tab(); shown != nil && shown.tool != nil && shown.tool.toolbox == page {
			a.focus = paneEditor
			if a.col != index {
				a.activate(col, col.active, false)
				a.invalidate()
			}
			return
		}
	}
}

func (a *app) closeToolboxTab(page *toolboxTab) {
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool != nil && tab.tool.toolbox == page {
				a.closeTab(tab, nil, nil)
				a.invalidate()
				return
			}
		}
	}
}

func (a *app) syncToolboxTabs(host *pluginHost) {
	var closing []*colTab
	for _, col := range a.cols {
		for _, tab := range col.tabs {
			if tab.tool == nil || tab.tool.toolbox == nil {
				continue
			}
			page := tab.tool.toolbox
			current := host.toolboxProvider(page.provider.Name)
			switch {
			case current == nil:
				closing = append(closing, tab)
			case current != page.provider:
				page.release()
				page.provider = current
			}
		}
	}
	if len(closing) > 0 {
		active := a.activeTab()
		for _, tab := range closing {
			a.dropTab(tab)
		}
		if active != a.activeTab() {
			a.focusColumn()
		}
		a.rememberTabs()
	}
}
