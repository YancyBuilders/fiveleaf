package main

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	"github.com/smallnest/uix/components/button"
)

type permissionPreview struct {
	question, target string
	complete         bool
}

func previewPermission(raw json.RawMessage, locations []agentLocation, title string, hasContent bool) permissionPreview {
	if len(raw) == 0 || string(raw) == "null" {
		if !hasContent && len(locations) == 1 && locations[0].Path != "" {
			target := locations[0].Path
			if locations[0].Line != nil {
				target += ":" + strconv.Itoa(*locations[0].Line)
			}
			return permissionPreview{target: target, complete: true}
		}
		return permissionPreview{}
	}
	var fields map[string]any
	if len(raw) > 8*1024 || json.Unmarshal(raw, &fields) != nil {
		return permissionPreview{}
	}
	soleInput := len(locations) == 0 && !hasContent
	message, _ := fields["message"].(string)
	if question := strings.TrimSpace(message); fields["method"] == "confirm" && question != "" {
		delete(fields, "method")
		delete(fields, "message")
		if rawTitle, ok := fields["title"].(string); ok && rawTitle == title {
			delete(fields, "title")
		}
		return permissionPreview{question: question, complete: soleInput && len(fields) == 0}
	}
	if soleInput && len(fields) == 1 {
		for _, key := range []string{"command", "cmd", "path", "file_path", "filePath"} {
			if value, ok := fields[key].(string); ok && strings.TrimSpace(value) != "" {
				return permissionPreview{target: value, complete: true}
			}
		}
	}
	return permissionPreview{}
}

func (request *agentPermission) optionLabel(option acp.PermissionOption) string {
	name := option.Name
	switch option.Kind {
	case "allow_once":
		if request.preview.question != "" && strings.EqualFold(name, "Yes") {
			return locales.AgentPermissionYes()
		}
		if strings.EqualFold(name, "Allow once") {
			return locales.AgentPermissionAllowOnce()
		}
	case "reject_once":
		if request.preview.question != "" && strings.EqualFold(name, "No") {
			return locales.AgentPermissionNo()
		}
		if strings.EqualFold(name, "Reject") {
			return locales.AgentPermissionReject()
		}
	}
	return cmp.Or(name, string(option.OptionId))
}

func (a *app) agentPermissionsView(c *ui.Context, chat *agentChat, pal *palette) {
	for _, request := range chat.permissions {
		if request != nil {
			a.permissionCard(c, chat, request, pal)
		}
	}
}

func (a *app) permissionCard(c *ui.Context, chat *agentChat, request *agentPermission, pal *palette) {
	m := pal.metricsOr()
	open := !request.preview.complete
	if request.expansionSet {
		open = request.expanded
	}
	answer := func(id string) {
		a.post(func() {
			if request.choose != nil && slices.Contains(chat.permissions, request) {
				choose := request.choose
				request.choose = nil
				choose(id)
			}
		})
	}
	card := ui.Column(c.Key(request)).WidthPercent(100).MaxWidth(480).MinWidth(0).AlignSelf(ui.Start).
		Role(ui.RoleGroup).Label(locales.AgentPermission()).Padding(m.inset+m.gap, 2*m.inset).
		Gap(2*m.gap).Radius(m.radiusCard).Border(1, pal.border).Background(pal.surface)
	compact := card.Bounds().W > 0 && card.Bounds().W < 240
	if compact {
		card.Padding(m.inset)
	}
	contextTitle := request.preview.question != "" && request.title != "" && request.title != request.preview.question
	card.Children(func() {
		ui.Row(c.Key("header")).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(m.pad).Children(func() {
			ui.Icon(c, iconLock).FontSize(14).TextColor(pal.modifiedFg).Shrink(0)
			ui.Text(c, locales.AgentTaskApproval()).FontSize(m.textSmall).FontWeight(m.weightBold).
				TextColor(pal.modifiedFg).SingleLine().MinWidth(0)
			if contextTitle && !compact {
				ui.Text(c, "·  "+request.title).FontSize(m.textSmall).TextColor(pal.muted).SingleLine().MinWidth(0).Tooltip(request.title)
			}
			ui.Spacer(c).MinWidth(0)
			label := locales.ButtonCancel()
			var cancel ui.Element
			if compact {
				cancel = a.iconButtonBase(c.Key("cancel"), iconX, label, false).Size(26, 26).Tooltip(label)
			} else {
				cancel = button.Button(c.Key("cancel"), button.Props{Label: label, Variant: button.Ghost, Size: button.Sm}).
					Label(label).MinHeight(28).FontSize(m.textSmall).Padding(0, m.pad).TextColor(pal.muted)
			}
			if cancel.Shrink(0).Disabled(request.choose == nil).Cursor(ui.CursorPointer).Clicked() {
				answer("")
			}
		})
		if contextTitle && compact {
			ui.Text(c, request.title).FillWidth().MinWidth(0).FontSize(m.textSmall).TextColor(pal.muted)
		}
		if heading := cmp.Or(request.preview.question, request.title); heading != "" {
			ui.Text(c, heading).FillWidth().MinWidth(0).FontSize(m.textTitle + 4).FontWeight(m.weightBold).
				LineHeight(1.35).TextColor(pal.text).Selectable()
		}
		if request.preview.target != "" {
			ui.Text(c, request.preview.target).FillWidth().MinWidth(0).Font(a.settings.FontFamily).
				FontSize(m.textBody).TextColor(pal.text).Selectable()
		}
		if open && request.detail != "" {
			a.permissionDetails(c, request, pal)
		}
		request.actions(c, pal, open, answer)
	})
}

func (a *app) permissionDetails(c *ui.Context, request *agentPermission, pal *palette) {
	m := pal.metricsOr()
	detail, clipped := agentVisibleText(strings.TrimSuffix(request.detail, "\n"), 8*1024)
	ui.Row(c.Key("raw")).FillWidth().MinWidth(0).Gap(m.gap).AlignItems(ui.Start).Children(func() {
		ui.ScrollBoth(c).Grow(1).MinWidth(0).MaxHeight(140).Padding(m.pad, m.inset).
			BorderWidth(0, 0, 0, 2).BorderColor(pal.border).Children(func() {
			ui.Text(c, detail).Font(a.settings.FontFamily).FontSize(m.textSmall).LineHeight(1.4).TextColor(pal.text).NoWrap().Selectable()
		})
		label := locales.AgentPermissionCopy()
		if a.iconButtonBase(c.Key("copy"), iconCopy, label, false).Size(26, 26).Shrink(0).Tooltip(label).Clicked() {
			c.WriteClipboard(request.detail)
		}
	})
	if clipped {
		ui.Text(c, locales.AgentMoreOutput()).FontSize(m.textSmall).TextColor(pal.muted)
	}
}

func (request *agentPermission) actions(c *ui.Context, pal *palette, open bool, answer func(string)) {
	m := pal.metricsOr()
	var persistent []acp.PermissionOption
	allowOnce := 0
	for _, option := range request.options {
		switch option.Kind {
		case "allow_always", "reject_always":
			persistent = append(persistent, option)
		case "allow_once":
			allowOnce++
		}
	}
	ui.Row(c.Key("decisions")).FillWidth().MinWidth(0).Wrap().AlignItems(ui.Center).Gap(m.pad).Margin(m.gap, 0, 0, 0).Children(func() {
		for _, option := range request.options {
			if option.Kind != "allow_always" && option.Kind != "reject_always" {
				request.optionButton(c, option, allowOnce == 1 && option.Kind == "allow_once", pal, answer)
			}
		}
		if request.detail == "" {
			return
		}
		ui.Spacer(c).MinWidth(0)
		toggle := ui.ButtonBase(c.Key("details")).Label(locales.AgentPermissionDetails()).Expanded(open).
			MinHeight(30).MaxWidthPercent(100).Padding(0, m.pad).Radius(m.radius).Gap(m.gap).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if toggle.Hovered() || toggle.FocusVisible() {
			toggle.Background(pal.hover)
		}
		toggle.Children(func() {
			icon := iconChevronRight
			if open {
				icon = iconChevronDown
			}
			ui.Icon(c, icon).FontSize(m.textSmall).TextColor(pal.muted).Shrink(0)
			ui.Text(c, locales.AgentPermissionDetails()).FontSize(m.textSmall).TextColor(pal.muted).MinWidth(0)
		})
		if toggle.Clicked() {
			request.expanded, request.expansionSet = !open, true
		}
	})
	if len(persistent) > 0 {
		ui.Column(c.Key("future")).FillWidth().MinWidth(0).Gap(m.pad).Padding(m.inset, 0, 0, 0).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Children(func() {
			ui.Text(c, locales.AgentPermissionFuture()).FontSize(m.textSmall).TextColor(pal.muted)
			ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(m.pad).Children(func() {
				for _, option := range persistent {
					request.optionButton(c, option, false, pal, answer)
				}
			})
		})
	}
}

func (request *agentPermission) optionButton(c *ui.Context, option acp.PermissionOption, primary bool, pal *palette, answer func(string)) {
	m := pal.metricsOr()
	makeButton := ui.Button
	if primary {
		makeButton = ui.PrimaryButton
	}
	label := request.optionLabel(option)
	button := makeButton(c.Key("option:"+string(option.OptionId)), "").Label(label).Disabled(request.choose == nil).
		MinWidth(52).MaxWidthPercent(100).MinHeight(34).Padding(m.pad, m.inset).Gap(m.gap).Radius(m.radius).
		AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
	button.Children(func() {
		if primary {
			ui.Icon(c, iconCheck).FontSize(14).Shrink(0)
		}
		ui.Text(c, label).FontSize(m.textBody).FontWeight(m.weightMedium).TextAlign(ui.Center).MinWidth(0)
	})
	if button.Clicked() {
		answer(string(option.OptionId))
	}
}
