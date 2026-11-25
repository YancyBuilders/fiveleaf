package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"fiveleaf/locales"
)

func (a *app) updateRequired() bool {
	return a.host != nil && a.host.update().minimum != ""
}

func updateMenuLabel(version string) string {
	if version != "" {
		return locales.MenuUpdateTo(version)
	}
	return locales.CmdHelpCheckForUpdates()
}

func (a *app) openUpdates() {
	update := a.host.update()
	if update.phase == updateDisabled {
		return
	}
	a.updatesOpen = update.minimum == ""
	if update.phase == updateIdle || update.phase == updateCurrent ||
		update.minimum != "" && (update.phase == updateInstallFailed || update.phase == updateInstalled) {
		a.host.checkForUpdates()
	}
	a.invalidate()
}

func (a *app) updateTitleButton(c *ui.Context, update updateStatus) {
	if update.phase == updateDisabled {
		return
	}
	label := locales.CmdHelpCheckForUpdates()
	switch {
	case update.minimum != "":
		label = locales.UpdateRequired()
	case update.version != "":
		label = locales.UpdateAvailable(update.version)
	}
	highlighted := update.version != "" || update.minimum != ""
	button := a.iconButton(c.Key("title:update"), iconUpdate, label, highlighted).Shrink(0)
	if highlighted && !button.Hovered() && !button.Pressed() {
		button.Background(a.pal.accent.Alpha(0.16))
	}
	if button.Clicked() {
		a.post(a.openUpdates)
	}
}

func (a *app) updateView(c *ui.Context, update updateStatus) {
	if !a.updatesOpen {
		return
	}
	ui.Overlay(c.Key("updates"), func() {
		back := a.dialogBack(c)
		back.Children(func() {
			panel := a.updatePanel(c, update).Margin(max(c.TitleBar().Height, titleBarH)+8, 0, 0, 0)
			if panel.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
				a.updatesOpen = false
			}
		})
	})
}

func (a *app) requiredUpdateView(c *ui.Context, update updateStatus) {
	ui.Column(c.Key("update-lock")).Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		a.updatePanel(c, update)
	})
}

func (a *app) updatePanel(c *ui.Context, update updateStatus) ui.Element {
	p := a.pal
	m := p.metricsOr()
	_, height := c.Size()
	required := update.minimum != ""
	title := locales.UpdateTitle()
	if required {
		title = locales.UpdateRequired()
	} else if update.phase == updateAvailable || update.phase == updateInstallFailed {
		title = locales.UpdateAvailable(update.version)
	}
	maxHeight := max(0, height-max(c.TitleBar().Height, titleBarH)-16)
	panel := ui.Column(c.Key("update-panel")).Width(520).MaxWidthPercent(94).MaxHeight(maxHeight).Padding(m.inset).Gap(10).Radius(m.radiusCard).Label(title)
	if !required {
		panel = a.floating(c, panel, true).Role(ui.RoleDialog)
	}
	panel.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Shrink(0).Children(func() {
			ui.Icon(c, iconUpdate).FontSize(18).TextColor(p.accent).Shrink(0)
			ui.Text(c, title).FontSize(m.textTitle + 1).FontWeight(m.weightBold).TextColor(p.text).MinWidth(0)
		})
		ui.Scroll(c.Key("update-details")).FillWidth().MinHeight(0).MaxHeight(310).Shrink(1).Children(func() {
			ui.Column(c).Gap(10).Children(func() {
				if required {
					ui.Text(c, locales.UpdateMinimum(update.minimum)).FontSize(m.textBody).TextColor(p.muted)
				}
				if text, failed := updateMessage(update); text != "" {
					color := p.muted
					if failed {
						color = p.dangerFg
					}
					ui.Text(c, text).FontSize(m.textBody).TextColor(color)
				}
				switch update.phase {
				case updateChecking:
					ui.Progress(c, -1).FillWidth().Height(5).Label(locales.UpdateChecking())
				case updateDownloading:
					ui.Progress(c, float64(update.progress)/100).FillWidth().Height(5).Label(locales.UpdateDownloading())
				}
				if update.version == "" || update.phase == updateInstalled {
					return
				}
				notes := strings.TrimSpace(update.notes)
				if notes == "" {
					ui.Text(c, locales.UpdateNoNotes()).FontSize(m.textBody).TextColor(p.muted)
					return
				}
				ui.Column(c.Key(update.version)).Gap(6).Padding(8, 8, 0, 0).BorderColor(p.border).BorderWidth(1, 0, 0, 0).Children(func() {
					ui.Text(c, locales.UpdateReleaseNotes()).FontSize(m.textBody).FontWeight(m.weightBold).TextColor(p.text)
					markdown.View(c, notes, nil).FillWidth().MinWidth(0)
				})
			})
		})
		a.updateActions(c, update)
	})
	return panel
}

func updateMessage(update updateStatus) (text string, failed bool) {
	switch update.phase {
	case updateDisabled:
		return locales.UpdateUnavailable(), true
	case updateChecking:
		return locales.UpdateChecking(), false
	case updateCurrent:
		return locales.UpdateCurrent(), false
	case updateDownloading:
		return locales.UpdateDownloading(), false
	case updateInstalled:
		if update.moreAfterRestart {
			return locales.UpdateContinueAfterRestart(update.version), false
		}
		return locales.UpdateReady(update.version), false
	case updateCheckFailed:
		return locales.UpdateCheckFailed(), true
	case updateInstallFailed:
		return locales.UpdateInstallFailed(), true
	}
	return "", false
}

func (a *app) updateActions(c *ui.Context, update updateStatus) {
	var label string
	var action func()
	switch update.phase {
	case updateIdle, updateCurrent:
		label, action = locales.UpdateRecheck(), a.host.checkForUpdates
	case updateCheckFailed:
		label, action = locales.UpdateRetry(), a.host.checkForUpdates
	case updateAvailable:
		label, action = locales.UpdateInstall(update.version), a.host.installUpdate
	case updateInstallFailed:
		label, action = locales.UpdateRetry(), a.host.installUpdate
	case updateInstalled:
		label, action = locales.UpdateRestart(), a.host.restartForUpdate
	}
	required := update.minimum != ""
	save := required && a.anyDirty()
	if !save && required && action == nil {
		return
	}
	ui.Row(c).Gap(8).Wrap().Shrink(0).Justify(ui.End).Children(func() {
		if save && ui.Button(c, locales.CmdFileSaveAll()).Clicked() {
			a.post(a.saveAll)
		}
		if !required {
			closeLabel := locales.ButtonClose()
			if update.phase == updateAvailable || update.phase == updateInstalled {
				closeLabel = locales.UpdateLater()
			}
			if ui.Button(c, closeLabel).Clicked() {
				a.updatesOpen = false
			}
		}
		if update.phase == updateInstallFailed && updatePolicyURL != "" && repoURL != "" && ui.Button(c, locales.UpdateDownloadGitHub()).Clicked() {
			a.post(func() { a.services.OpenURL(repoURL + "/releases/latest") })
		}
		if (update.phase == updateInstallFailed || required && update.phase == updateInstalled) && ui.Button(c, locales.UpdateRecheck()).Clicked() {
			a.post(a.host.checkForUpdates)
		}
		if action != nil && ui.PrimaryButton(c, label).AutoFocus().Clicked() {
			a.post(action)
		}
	})
}
