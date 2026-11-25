package main

import (
	"cmp"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type prefAgentVariable struct{ name, value string }

// prefAgentDraft is separate from settings until Save. The original definition
// detects edits made in another window or in settings.json meanwhile.
type prefAgentDraft struct {
	id, name, command string
	args              []string
	env               []prefAgentVariable
	protocol, cwd     string
	capabilities      string
	timeout           string
	disabled          bool
	promptArgument    bool
	original          settings.AgentServer
	existing, focus   bool
	err, field        string
}

func cloneAgentServer(server settings.AgentServer) settings.AgentServer {
	server.Args, server.Env = slices.Clone(server.Args), maps.Clone(server.Env)
	server.Capabilities = slices.Clone(server.Capabilities)
	return server
}

// pluginAgentIDs includes disabled plugins so a later toggle cannot silently
// turn a custom agent into an override.
func pluginAgentIDs() map[string]bool {
	ids := map[string]bool{}
	if plugins != nil {
		for _, p := range plugins.reg.Plugins() {
			for _, server := range p.Contributes.AgentServers {
				ids[server.ID] = true
			}
		}
	}
	return ids
}

func customAgentID(name string, servers map[string]settings.AgentServer, reserved map[string]bool) string {
	var slug strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			slug.WriteRune(r)
		} else if slug.Len() > 0 && !strings.HasSuffix(slug.String(), "-") {
			slug.WriteByte('-')
		}
		if slug.Len() >= 48 {
			break
		}
	}
	base := "custom-" + cmp.Or(strings.Trim(slug.String(), "-"), "agent")
	id := base
	for n := 2; ; n++ {
		_, exists := servers[id]
		if !exists && !reserved[id] {
			return id
		}
		id = base + "-" + strconv.Itoa(n)
	}
}

func (w *prefsWindow) editAgentServer(id string) {
	if w.agentConfigurationEditing() || w.agentIOBusy {
		return
	}
	d := &prefAgentDraft{focus: true, timeout: strconv.Itoa(settings.DefaultAgentTimeoutSeconds)}
	if id != "" {
		server, ok := w.m.s.AgentServers[id]
		if !ok {
			return
		}
		d.id, d.name, d.command = id, cmp.Or(server.Name, id), server.Command
		d.existing, d.original, d.args = true, cloneAgentServer(server), slices.Clone(server.Args)
		d.protocol, d.cwd, d.capabilities = server.Protocol, server.WorkingDirectory, strings.Join(server.Capabilities, ", ")
		d.timeout, d.disabled, d.promptArgument = strconv.Itoa(server.ReplyTimeoutSeconds()), server.Disabled, server.PromptArgument
		for _, name := range slices.Sorted(maps.Keys(server.Env)) {
			d.env = append(d.env, prefAgentVariable{name, server.Env[name]})
		}
	}
	w.agentDraft = d
}

func (d *prefAgentDraft) definition() (settings.AgentServer, bool) {
	d.err, d.field = "", ""
	server := settings.AgentServer{
		Name: strings.TrimSpace(d.name), Command: strings.TrimSpace(d.command), Args: slices.Clone(d.args),
		Protocol: d.protocol, WorkingDirectory: d.cwd, Disabled: d.disabled, PromptArgument: d.promptArgument,
	}
	if server.Name == "" || strings.ContainsAny(server.Name, "\x00\r\n") {
		d.err, d.field = locales.PrefsAgentNameRequired(), "name"
		return server, false
	}
	if server.Command == "" {
		d.err, d.field = locales.PrefsAgentCommandRequired(), "command"
		return server, false
	}
	timeout, err := strconv.Atoi(strings.TrimSpace(d.timeout))
	if err != nil || timeout < 1 || timeout > 3600 {
		d.err, d.field = locales.PrefsAgentTimeoutInvalid(), "timeout"
		return server, false
	}
	if timeout != settings.DefaultAgentTimeoutSeconds || d.original.TimeoutSeconds != 0 {
		server.TimeoutSeconds = timeout
	}
	if strings.TrimSpace(d.capabilities) != "" {
		for _, tag := range strings.Split(d.capabilities, ",") {
			server.Capabilities = append(server.Capabilities, strings.TrimSpace(tag))
		}
	}
	if len(d.env) > 0 {
		server.Env = map[string]string{}
	}
	for _, variable := range d.env {
		if variable.name == "" || strings.ContainsAny(variable.name, "=\x00") || strings.IndexFunc(variable.name, unicode.IsSpace) >= 0 {
			d.err, d.field = locales.PrefsAgentEnvInvalid(), "env"
			return server, false
		}
		if _, exists := server.Env[variable.name]; exists {
			d.err, d.field = locales.PrefsAgentDuplicateEnv(), "env"
			return server, false
		}
		server.Env[variable.name] = variable.value
	}
	if err := server.Validate(); err != nil {
		d.err = err.Error()
		return server, false
	}
	return server, true
}

func (w *prefsWindow) saveAgentServer() bool {
	d := w.agentDraft
	if d == nil {
		return false
	}
	server, valid := d.definition()
	if !valid {
		return false
	}
	servers := maps.Clone(w.m.s.AgentServers)
	if servers == nil {
		servers = map[string]settings.AgentServer{}
	}
	id := d.id
	if d.existing {
		current, exists := servers[id]
		if !exists || !reflect.DeepEqual(current, d.original) {
			d.err = locales.PrefsAgentConflict()
			return false
		}
	} else {
		id = customAgentID(server.Name, servers, pluginAgentIDs())
	}
	servers[id] = server
	if !w.applyAgentServers(servers) {
		return false
	}
	w.agentDraft = nil
	return true
}

func (w *prefsWindow) applyAgentServers(servers map[string]settings.AgentServer) bool {
	raw, err := json.Marshal(servers)
	if err == nil {
		prefs := w.m.s.Prefs
		err = prefs.Set("agentServers", raw)
	}
	if err != nil {
		if w.agentDraft != nil {
			w.agentDraft.err = err.Error()
		} else {
			w.errors = append(w.errors, err.Error())
		}
		return false
	}
	w.set("agentServers", raw)
	return true
}

func (w *prefsWindow) removeAgentServer(id string) {
	server, exists := w.m.s.AgentServers[id]
	if !exists {
		return
	}
	server = cloneAgentServer(server)
	servers := maps.Clone(w.m.s.AgentServers)
	delete(servers, id)
	if !w.applyAgentServers(servers) {
		return
	}
	if d := w.agentDraft; d != nil && d.existing && d.id == id {
		w.agentDraft = nil
	}
	w.undoable("agent-remove:"+id, locales.PrefsAgentRemoved(cmp.Or(server.Name, id)), func() {
		current := maps.Clone(w.m.s.AgentServers)
		if _, exists := current[id]; exists {
			w.errors = append(w.errors, locales.PrefsAgentUndoConflict())
			return
		}
		if current == nil {
			current = map[string]settings.AgentServer{}
		}
		current[id] = server
		w.applyAgentServers(current)
	})
}

// Undo restores only definitions removed by this reset. Agents registered or
// changed afterward remain intact, including edits from settings.json.
func (w *prefsWindow) resetAgentServers() {
	removed := make(map[string]settings.AgentServer, len(w.m.s.AgentServers))
	for id, server := range w.m.s.AgentServers {
		removed[id] = cloneAgentServer(server)
	}
	if !w.applyAgentServers(settings.Default().AgentServers) {
		return
	}
	w.undoable("reset:agentServers", locales.PrefsWasReset(prefTitle("agentServers")), func() {
		current := maps.Clone(w.m.s.AgentServers)
		for id := range removed {
			if _, exists := current[id]; exists {
				w.errors = append(w.errors, locales.PrefsAgentUndoConflict())
				return
			}
		}
		if current == nil {
			current = map[string]settings.AgentServer{}
		}
		maps.Copy(current, removed)
		w.applyAgentServers(current)
	})
}

func (w *prefsWindow) agentServersControl(c *ui.Context) {
	p := w.pal
	ui.Column(c.Key("prefs-agents")).FillWidth().Children(func() {
		if w.agentDraft != nil {
			w.agentServerForm(c)
			return
		}
		providers := (&app{settings: w.m.s}).agentProviders()
		reserved := pluginAgentIDs()
		if len(providers) == 0 {
			ui.Text(c, locales.PrefsAgentEmpty()).FontSize(12).TextColor(p.muted)
		}
		disabled := w.agentConfigurationEditing() || w.agentIOBusy
		for index, provider := range providers {
			row := ui.Row(c.Key(provider.id)).Gap(8).AlignItems(ui.Center).Padding(6, 2)
			w.rowHover(row, index > 0)
			row.Children(func() {
				w.tileOf(c, "", provider.chatMark(p), false, 30)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, provider.name).FontSize(13).FontWeight(500).TextColor(p.text).SingleLine()
					detail := strings.ToUpper(provider.config.EffectiveProtocol()) + " · " + provider.id
					if provider.config.Disabled {
						detail += " · " + locales.PrefsAgentDisabled()
					}
					ui.Text(c, detail).Font(w.monoFont()).FontSize(10.5).TextColor(p.muted).SingleLine().Tooltip(provider.config.Command)
				})
				if provider.plugin != "" {
					w.pill(c, "source", locales.RunningColPlugin(), p.muted, false)
					if w.iconButton(c, "plugin", iconExternal, locales.PrefsAgentOpenPlugin(provider.plugin), p.accent, disabled) {
						w.navigate(prefsPlugins, pluginDetailKey+provider.plugin, nil)
					}
					return
				}
				source := locales.PrefsAgentCustomSource()
				if reserved[provider.id] {
					source = locales.PrefsAgentOverridesPlugin()
				}
				w.pill(c, "source", source, p.muted, false)
				if w.iconButton(c, "edit", iconPencil, locales.PrefsAgentEdit()+": "+provider.name, p.accent, disabled) {
					w.editAgentServer(provider.id)
				}
				if w.iconButton(c, "remove", iconTrash, locales.PrefsAgentRemove()+": "+provider.name, p.dangerFg, disabled) {
					w.removeAgentServer(provider.id)
				}
			})
		}
		compactActionRow(c, ui.Start).Padding(8, 0, 0, 0).Children(func() {
			if accentActionButton(c, locales.PrefsAgentAdd(), iconPlus, p.accent).Disabled(disabled).Clicked() {
				w.editAgentServer("")
			}
			if actionButton(c, locales.PrefsAgentImport(), uiicons.Download()).Disabled(w.agentConfigurationEditing() || w.agentIOBusy).Clicked() {
				w.importAgentConfiguration(agentFile)
			}
			if actionButton(c, locales.PrefsAgentExport(), uiicons.Upload()).Disabled(w.agentConfigurationEditing() || w.agentIOBusy).Clicked() {
				w.exportAgentConfiguration(agentFile)
			}
			if actionButton(c, locales.PrefsEditInJSON(), iconPencil).Clicked() {
				w.inFront(func(a *app) { a.openSettings() })
			}
		})
	})
}

func (w *prefsWindow) agentRowButton(c *ui.Context, key string, icon *ui.SVG, label string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(26, 26).Radius(4).AlignItems(ui.Center).Justify(ui.Center).Label(label).Tooltip(label).Shrink(0)
	if b.Hovered() {
		b.Background(w.pal.hover)
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(13).TextColor(w.pal.muted) })
	return b
}

func (w *prefsWindow) agentServerForm(c *ui.Context) {
	d, p := w.agentDraft, w.pal
	ui.Column(c.Key("prefs-agent-form")).Gap(10).Padding(8, 0).Children(func() {
		title := locales.PrefsAgentAdd()
		if d.existing {
			title = locales.PrefsAgentEdit()
		}
		ui.Text(c, strings.TrimSuffix(title, "…")).FontSize(13).FontWeight(600).TextColor(p.text)
		field := func(key, label string, value *string) ui.Element {
			var input ui.Element
			ui.Column(c.Key(key)).Gap(4).Children(func() {
				ui.Text(c, label).FontSize(11.5).TextColor(p.muted)
				input = ui.TextInput(c.Key("input"), value).FillWidth().FontSize(12).Label(label)
				if d.field == key && d.err != "" {
					input.Error(d.err)
				}
			})
			return input
		}
		name := field("name", locales.PrefsAgentName(), &d.name)
		if d.focus {
			name.Focus().ScrollIntoView()
			d.focus = false
		}
		id := d.id
		if !d.existing {
			id = customAgentID(d.name, w.m.s.AgentServers, pluginAgentIDs())
		}
		ui.Text(c, locales.PrefsAgentID()+": "+id).FontSize(11).TextColor(p.faint).Selectable()
		field("command", locales.PrefsAgentCommand(), &d.command)
		ui.Column(c.Key("protocol")).Gap(4).Children(func() {
			ui.Text(c, locales.PrefsAgentProtocol()).FontSize(11.5).TextColor(p.muted)
			chosen := strings.ToUpper(cmp.Or(d.protocol, "acp"))
			if ui.Select(c, &chosen, []string{"ACP", "CLI"}).FillWidth().FontSize(12).Label(locales.PrefsAgentProtocol()).Changed() {
				d.protocol = strings.ToLower(chosen)
				if d.protocol != "cli" {
					d.promptArgument = false
				}
			}
		})
		ui.Column(c.Key("args")).Gap(5).Children(func() {
			ui.Text(c, locales.PrefsAgentArguments()).FontSize(11.5).TextColor(p.muted)
			remove := -1
			for i := range d.args {
				ui.Row(c.Key(i)).Gap(5).AlignItems(ui.Start).Children(func() {
					ui.TextArea(c, &d.args[i]).Grow(1).MinWidth(0).FontSize(12).Height(agentValueHeight(d.args[i])).Label(locales.PrefsAgentArgument() + " " + strconv.Itoa(i+1))
					if w.agentRowButton(c, "remove", iconX, locales.PrefsAgentRemoveArgument()+" "+strconv.Itoa(i+1)).Clicked() {
						remove = i
					}
				})
			}
			if remove >= 0 {
				d.args = slices.Delete(d.args, remove, remove+1)
			}
			if actionButton(c, locales.PrefsAgentAddArgument(), iconPlus).AlignSelf(ui.Start).Clicked() {
				d.args = append(d.args, "")
			}
		})
		ui.Column(c.Key("environment")).Gap(5).Children(func() {
			ui.Text(c, locales.PrefsAgentEnvironment()).FontSize(11.5).TextColor(p.muted)
			remove := -1
			for i := range d.env {
				variable := &d.env[i]
				ui.Row(c.Key(i)).Gap(5).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
						ui.TextInput(c, &variable.name).FillWidth().FontSize(12).Placeholder(locales.PrefsAgentEnvName()).Label(locales.PrefsAgentEnvName() + " " + strconv.Itoa(i+1))
						ui.TextArea(c, &variable.value).FillWidth().FontSize(12).Height(agentValueHeight(variable.value)).Placeholder(locales.PrefsAgentEnvValue()).Label(locales.PrefsAgentEnvValue() + " " + strconv.Itoa(i+1))
					})
					if w.agentRowButton(c, "remove", iconX, locales.PrefsAgentRemoveEnv()+" "+strconv.Itoa(i+1)).Clicked() {
						remove = i
					}
				})
			}
			if remove >= 0 {
				d.env = slices.Delete(d.env, remove, remove+1)
			}
			if actionButton(c, locales.PrefsAgentAddEnv(), iconPlus).AlignSelf(ui.Start).Clicked() {
				d.env = append(d.env, prefAgentVariable{})
			}
		})
		ui.Column(c.Key("connection")).Gap(10).Children(func() {
			field("workingDirectory", locales.PrefsAgentWorkingDirectory(), &d.cwd).Placeholder(locales.PrefsAgentProjectDirectory())
			field("capabilities", locales.PrefsAgentCapabilities(), &d.capabilities).Placeholder(locales.PrefsAgentCapabilitiesPlaceholder())
			field("timeout", locales.PrefsAgentTimeout(), &d.timeout)
			ui.Row(c.Key("disabled")).Gap(8).AlignItems(ui.Center).Children(func() {
				w.toggle(c, &d.disabled).Label(locales.PrefsAgentDisabled())
				ui.Text(c, locales.PrefsAgentDisabled()).FontSize(12).TextColor(p.text)
			})
			if d.protocol == "cli" {
				ui.Row(c.Key("promptArgument")).Gap(8).AlignItems(ui.Center).Children(func() {
					w.toggle(c, &d.promptArgument).Label(locales.PrefsAgentPromptArgument())
					ui.Text(c, locales.PrefsAgentPromptArgument()).FontSize(12).TextColor(p.text).Grow(1).MinWidth(0)
				})
			}
		})
		if d.err != "" {
			ui.Text(c, d.err).FontSize(12).TextColor(p.dangerFg).Selectable()
		}
		compactActionRow(c, ui.End).Children(func() {
			if actionButton(c, locales.ButtonCancel(), nil).Clicked() {
				w.agentDraft = nil
			}
			if w.primaryButton(c, "save", locales.ButtonSave(), false).Clicked() {
				w.saveAgentServer()
			}
		})
	})
}

func agentValueHeight(value string) float32 {
	return min(float32(36+strings.Count(value, "\n")*16), 100)
}
