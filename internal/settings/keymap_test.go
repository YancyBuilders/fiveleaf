package settings

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestKeymapPreference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	if s.Keymap != "" || !s.IsDefaultKey("keymap") {
		t.Fatal("default keymap is not FiveLeaf")
	}
	for _, id := range []string{"vscode", "jetbrains", "sublime", "example.keys"} {
		raw, _ := json.Marshal(id)
		if err := s.Prefs.Set("keymap", raw); err != nil {
			t.Fatal(err)
		}
		s.Changed()
		s.Flush()
		if got := Load(path).Keymap; got != id {
			t.Fatalf("saved %q as %q", id, got)
		}
	}
	s.Reset("keymap")
	s.Changed()
	s.Flush()
	if got := Load(path).Keymap; got != "" {
		t.Fatalf("reset keymap %q", got)
	}
}
