package settings

import "testing"

func TestCopilotPreference(t *testing.T) {
	prefs := Default().Prefs
	want := CopilotConfig{Agent: "local", Model: "provider/model"}
	if err := prefs.Set("copilot", []byte(`{"agent":"local","model":"provider/model"}`)); err != nil || prefs.Copilot != want {
		t.Fatalf("set Copilot = %+v, %v", prefs.Copilot, err)
	}
	for _, raw := range []string{`{"model":"orphan"}`, `{"agent":"agent id"}`, `{"agent":"local","model":" bad "}`} {
		if err := prefs.Set("copilot", []byte(raw)); err == nil || prefs.Copilot != want {
			t.Fatalf("invalid selection changed Copilot: %s, %+v, %v", raw, prefs.Copilot, err)
		}
	}
	folder := LoadFolder(t.TempDir())
	if err := folder.Set("copilot", []byte(`{"agent":"other"}`)); err != nil {
		t.Fatal(err)
	}
	if folder.Has("copilot") || folder.Apply(prefs).Copilot != want {
		t.Fatal("folder settings could redirect Copilot requests")
	}
	prefs.Reset("copilot")
	if prefs.Copilot != (CopilotConfig{}) {
		t.Fatal("reset kept a configured route")
	}
}
