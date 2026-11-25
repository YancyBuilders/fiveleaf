package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"fiveleaf/internal/cli"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func TestRoleFileAndBatchAPIShareLiveSettings(t *testing.T) {
	ph := prefsRoleHarness(t)
	if !ph.pt.HasText(locales.PrefsRoleImport()) || !ph.pt.HasText(locales.PrefsRoleExport()) {
		t.Fatal("role list omitted its import or export button")
	}
	ph.m.s.AgentServers["local"] = settings.AgentServer{Command: "never-start", Protocol: "cli"}
	ask := func(action string, bundle []byte) cli.Reply {
		var response cli.Reply
		ph.m.serve(cli.Request{Roles: &cli.RoleRequest{Action: action, Bundle: bundle}}, func(reply cli.Reply) { response = reply })
		return response
	}
	file := []byte(`{"version":1,"roles":{"设计助手":{"followCurrentAgent":true,"prompt":"检查焦点"},"审稿助手":{"agent":"local"}}}`)
	for index, expected := range []string{`{"created":2,"presetsApplied":0}`, `{"created":0,"presetsApplied":0}`} {
		if result := ask("import", file); result.Error != "" || string(result.Data) != expected {
			t.Fatalf("import %d: %+v", index, result)
		}
	}
	ph.frames()
	if !ph.pt.HasText("设计助手") || roundtableInvitation(t, ph.a, "role:设计助手").role.Prompt != "检查焦点" {
		t.Fatal("API creations did not reach settings and roundtable invitations")
	}
	ph.m.s.Flush()
	if stored := settings.Load(ph.m.s.Path()); !stored.AgentRoles["设计助手"].FollowCurrentAgent {
		t.Fatal("API creation did not persist in the profile")
	}
	response := ask("export", nil)
	var exported struct {
		Presets map[string]settings.RolePresetTemplate
		Roles   map[string]settings.AgentRole
	}
	if err := json.Unmarshal(response.Data, &exported); err != nil || len(exported.Presets) != len(agentRolePresets) || len(exported.Roles) != 2 || bytes.Contains(response.Data, []byte("never-start")) {
		t.Fatalf("invalid API export: %v, %s", err, response.Error)
	}
	gui, err := roleFile.write(ph.m.s.Prefs)
	if err != nil || !bytes.Equal(gui, response.Data) {
		t.Fatal("the settings export and API export differ")
	}
	ph.w.finishAgentImport(roleFile, "roles.json", gui, nil)
	if len(ph.w.errors) != 0 || len(ph.w.notes) != 1 {
		t.Fatalf("the role list rejected its exported file: %v", ph.w.errors)
	}
	if result := ask("import", []byte(`{"version":1,"roles":{"new":{"agent":"local"},"审稿助手":{"agent":"other"}}}`)); result.Error == "" || len(ph.m.s.AgentRoles) != 2 {
		t.Fatal("a batch with a conflict was acknowledged or partially created")
	}
}
