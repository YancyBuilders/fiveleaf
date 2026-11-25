package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

type agentFileFormat struct {
	filename                 string
	importTitle, exportTitle func() string
	read                     func(*settings.Prefs, []byte) error
	write                    func(settings.Prefs) ([]byte, error)
}

var agentFile = agentFileFormat{"fiveleaf-agents.json", locales.PrefsAgentImportTitle, locales.PrefsAgentExportTitle, (*settings.Prefs).ImportAgents, settings.Prefs.ExportAgents}
var roleFile = agentFileFormat{"fiveleaf-roles.json", locales.PrefsRoleImport, locales.PrefsRoleExport, (*settings.Prefs).ImportRoles, exportRoleBundle}

func exportRoleBundle(prefs settings.Prefs) ([]byte, error) {
	templates := make(map[string]settings.RolePresetTemplate, len(agentRolePresets))
	for _, preset := range agentRolePresets {
		templates[preset.id] = settings.RolePresetTemplate{Name: preset.name(), DefaultPrompt: preset.prompt()}
	}
	return prefs.ExportRoles(templates)
}

func (w *prefsWindow) importAgentConfiguration(format agentFileFormat) {
	if w.agentIOBusy || w.agentConfigurationEditing() {
		return
	}
	w.agentIOBusy = true
	parent := w.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: format.importTitle(), ButtonLabel: locales.DialogOpen(),
			Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}},
		})
		var path string
		var raw []byte
		if err == nil && len(paths) > 0 {
			path = paths[0]
			raw, err = readAgentConfiguration(path)
		}
		w.post(func() { w.finishAgentImport(format, path, raw, err) })
	}()
}

func readAgentConfiguration(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, settings.MaxAgentConfigurationSize+1))
}

func (m *fiveleaf) importAgentFile(format agentFileFormat, raw []byte) error {
	prefs := m.s.Prefs
	if err := format.read(&prefs, raw); err != nil {
		return err
	}
	if len(prefs.AgentServers) != len(m.s.AgentServers) || len(prefs.AgentRoles) != len(m.s.AgentRoles) || len(prefs.AgentRolePresets) != len(m.s.AgentRolePresets) {
		m.reload(prefs)
		m.s.Changed()
	}
	return nil
}

func (w *prefsWindow) finishAgentImport(format agentFileFormat, path string, raw []byte, err error) {
	w.agentIOBusy = false
	if err == nil && path == "" {
		return
	}
	if err == nil {
		// Merge against the current registrations, including changes made
		// while the native dialog was open or the file was being read.
		err = w.m.importAgentFile(format, raw)
	}
	if err != nil {
		message := locales.FailOpen(filepath.Base(path), err.Error())
		if path == "" {
			message = locales.FailWithError(format.importTitle(), err.Error())
		}
		w.errors = append(w.errors, message)
		return
	}
	w.notes = append(w.notes, ui.Toast{ID: "imported:" + format.filename, Title: locales.PrefsAgentImported(filepath.Base(path))})
}

func (w *prefsWindow) exportAgentConfiguration(format agentFileFormat) {
	if w.agentIOBusy || w.agentConfigurationEditing() {
		return
	}
	raw, err := format.write(w.m.s.Prefs)
	if err != nil {
		w.errors = append(w.errors, err.Error())
		return
	}
	w.agentIOBusy = true
	parent := w.win
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: parent, Title: format.exportTitle(), ButtonLabel: locales.DialogExport(),
			DefaultPath: format.filename, CreateDirectories: true,
			Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}},
		})
		if err == nil && path != "" {
			err = writePrivateExport(path, bytes.NewReader(raw))
		}
		w.post(func() { w.finishAgentExport(format, path, err) })
	}()
}

func (w *prefsWindow) finishAgentExport(format agentFileFormat, path string, err error) {
	w.agentIOBusy = false
	if err != nil {
		message := locales.FailSave(filepath.Base(path), err.Error())
		if path == "" {
			message = locales.FailWithError(format.exportTitle(), err.Error())
		}
		w.errors = append(w.errors, message)
		return
	}
	if path != "" {
		w.notes = append(w.notes, ui.Toast{ID: "exported:" + format.filename, Title: locales.PrefsAgentExported(filepath.Base(path))})
	}
}
