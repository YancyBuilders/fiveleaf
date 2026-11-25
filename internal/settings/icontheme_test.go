package settings

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestIconThemePreference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	if !s.Prefs.IsDefaultKey("iconTheme") {
		t.Fatal("file icons should default to FiveLeaf")
	}
	s.Theme, s.Appearance = "onedarkpro", "light"
	if err := s.Prefs.Set("iconTheme", json.RawMessage(`"my-icons.blue"`)); err != nil {
		t.Fatal(err)
	}
	s.Changed()
	s.Flush()
	loaded := Load(path)
	if loaded.IconTheme != "my-icons.blue" || loaded.Theme != "onedarkpro" || loaded.Appearance != "light" {
		t.Fatalf("icon selection did not persist independently: %q, %q, %q", loaded.IconTheme, loaded.Theme, loaded.Appearance)
	}
	loaded.Prefs.Reset("iconTheme")
	if loaded.IconTheme != "" || loaded.Theme != "onedarkpro" || loaded.Appearance != "light" {
		t.Fatal("resetting icons changed the color theme or appearance")
	}
}
