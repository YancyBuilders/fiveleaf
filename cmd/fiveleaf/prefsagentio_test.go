package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestPrefsAgentImportMergesCurrentSettingsAndPersists(t *testing.T) {
	ph := prefsAgentHarness(t)
	if !ph.pt.HasText(locales.PrefsAgentImport()) || !ph.pt.HasText(locales.PrefsAgentExport()) {
		t.Fatal("agent configuration import and export controls are missing")
	}
	ph.w.agentIOBusy = true
	// This registration arrives after the native dialog was opened.
	ph.m.s.AgentServers["other"] = settings.AgentServer{Command: "other-agent"}
	ph.m.s.AgentRoles["Other role"] = settings.AgentRole{Agent: "other", Prompt: "keep"}
	source := settings.Default().Prefs
	want := settings.AgentServer{
		Name: "Imported CLI", Command: "cli-agent", Args: []string{"--print", ""}, Env: map[string]string{"PROFILE": "private"},
		Protocol: "cli", WorkingDirectory: "tools", Capabilities: []string{"search"}, Disabled: true, TimeoutSeconds: 60, PromptArgument: true,
	}
	source.AgentServers["imported"] = want
	wantRole := settings.AgentRole{Agent: "imported", Model: "imported-model", Prompt: "review this"}
	source.AgentRoles["Imported role"] = wantRole
	raw, err := source.ExportAgents()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agents.json")
	if err := writePrivateExport(path, bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	read, err := readAgentConfiguration(path)
	if err != nil || !bytes.Equal(read, raw) {
		t.Fatalf("could not read configuration: %v", err)
	}
	ph.w.finishAgentImport(agentFile, path, read, nil)
	if ph.w.agentIOBusy || len(ph.w.errors) != 0 || len(ph.w.notes) != 1 {
		t.Fatalf("import failed: %v", ph.w.errors)
	}
	ph.frames()
	ph.m.s.Flush()
	for _, p := range []settings.Prefs{ph.m.s.Prefs, ph.a.settings.Prefs, settings.Load(ph.m.s.Path()).Prefs} {
		if len(p.AgentServers) != 2 || p.AgentServers["other"].Command != "other-agent" || !reflect.DeepEqual(p.AgentServers["imported"], want) {
			t.Fatalf("import lost registrations or failed to propagate: %+v", p.AgentServers)
		}
		if len(p.AgentRoles) != 2 || p.AgentRoles["Other role"].Agent != "other" || p.AgentRoles["Imported role"] != wantRole {
			t.Fatalf("import lost roles or failed to propagate: %+v", p.AgentRoles)
		}
	}
	if ph.a.agent.client != nil || ph.a.agent.connecting || len(ph.a.agent.messages) != 0 {
		t.Fatal("importing configuration started an agent")
	}
}

func TestPrefsAgentImportRoleConflictIsAtomicAcrossWindows(t *testing.T) {
	for _, conflict := range []string{"agent", "role"} {
		t.Run(conflict, func(t *testing.T) {
			ph := prefsAgentHarness(t)
			w := ph.w
			// A second settings surface changes the same shared preferences while
			// the first window's native import dialog is open.
			other := newPrefsWindow(ph.m)
			w.agentIOBusy = true
			if !other.applyAgentServers(map[string]settings.AgentServer{"keep": {Command: "original"}}) ||
				!other.applyRoleSetting("agentRoles", map[string]settings.AgentRole{"Keep role": {Agent: "keep", Prompt: "original"}}) {
				t.Fatal("could not prepare the concurrent changes")
			}
			beforeAgents := bytes.Clone(ph.m.s.Raw("agentServers"))
			beforeRoles := bytes.Clone(ph.m.s.Raw("agentRoles"))
			source := settings.Default().Prefs
			source.AgentServers["new-agent"] = settings.AgentServer{Command: "new-agent"}
			source.AgentRoles["New role"] = settings.AgentRole{Agent: "new-agent"}
			if conflict == "agent" {
				source.AgentServers["keep"] = settings.AgentServer{Command: "replacement"}
			} else {
				source.AgentRoles["Keep role"] = settings.AgentRole{Agent: "new-agent", Prompt: "replacement"}
			}
			raw, err := source.ExportAgents()
			if err != nil {
				t.Fatal(err)
			}
			w.finishAgentImport(agentFile, "roles.json", raw, nil)
			if w.agentIOBusy || len(w.errors) != 1 || len(w.notes) != 0 ||
				!bytes.Equal(beforeAgents, ph.m.s.Raw("agentServers")) || !bytes.Equal(beforeRoles, ph.m.s.Raw("agentRoles")) {
				t.Fatal("conflicting joint import changed settings or reported success")
			}
			ph.m.s.Flush()
			reloaded := settings.Load(ph.m.s.Path())
			defer reloaded.Flush()
			for _, p := range []settings.Prefs{ph.a.settings.Prefs, reloaded.Prefs} {
				if !bytes.Equal(beforeAgents, p.Raw("agentServers")) || !bytes.Equal(beforeRoles, p.Raw("agentRoles")) {
					t.Fatal("failed joint import partially propagated or persisted")
				}
			}
		})
	}
}

func TestPrefsAgentImportConflictCancelAndFailures(t *testing.T) {
	ph := prefsAgentHarness(t)
	w := ph.w
	ph.m.s.AgentServers["keep"] = settings.AgentServer{Command: "original"}
	before := bytes.Clone(ph.m.s.Raw("agentServers"))
	w.agentIOBusy = true
	w.finishAgentImport(agentFile, "", nil, nil)
	w.agentIOBusy = true
	w.finishAgentExport(agentFile, "", nil)
	if w.agentIOBusy || len(w.errors) != 0 || len(w.notes) != 0 {
		t.Fatal("cancelled configuration dialog reported a result or stayed busy")
	}
	w.finishAgentImport(agentFile, "conflict.json", []byte(`{"version":1,"agentServers":{"new":{"command":"new"},"keep":{"command":"replacement"}}}`), nil)
	if len(w.errors) != 1 || len(w.notes) != 0 || !bytes.Equal(before, ph.m.s.Raw("agentServers")) {
		t.Fatal("conflicting import partially changed registrations or did not report an error")
	}
	w.finishAgentImport(agentFile, "broken.json", []byte(`{"version":1}`), nil)
	w.finishAgentExport(agentFile, "unwritable.json", errors.New("disk unavailable"))
	if len(w.errors) != 3 || len(w.notes) != 0 || !bytes.Equal(before, ph.m.s.Raw("agentServers")) {
		t.Fatal("failed import/export changed registrations or reported success")
	}
	path := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte(" "), settings.MaxAgentConfigurationSize*2), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := readAgentConfiguration(path)
	if err != nil || len(raw) != settings.MaxAgentConfigurationSize+1 {
		t.Fatal("configuration read was not bounded")
	}
	w.finishAgentImport(agentFile, path, raw, nil)
	w.finishAgentImport(agentFile, "", nil, errors.New("dialog unavailable"))
	w.finishAgentExport(agentFile, "", errors.New("dialog unavailable"))
	if len(w.errors) != 6 || !bytes.Equal(before, ph.m.s.Raw("agentServers")) ||
		w.errors[4] != locales.FailWithError(locales.PrefsAgentImportTitle(), "dialog unavailable") ||
		w.errors[5] != locales.FailWithError(locales.PrefsAgentExportTitle(), "dialog unavailable") {
		t.Fatal("file or dialog error lost its action or changed settings")
	}
}

func TestPrefsAgentExportUsesPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(path, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"version":1,"agentServers":{"agent":{"command":"tool","env":{"TOKEN":"private-value"}}}}`)
	if err := writePrivateExport(path, bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("export contents differ: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("configuration with explicit environment has mode %o", info.Mode().Perm())
	}
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateExport(blocked, bytes.NewReader(raw)); err == nil {
		t.Fatal("export replaced a directory")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 2 {
		t.Fatalf("failed export left a temporary file containing private values: %v", err)
	}
}
