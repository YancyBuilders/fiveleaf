package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFolderPathLegacy(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, ".fiveleaf", "settings.json")
	legacy := filepath.Join(dir, ".mayoi", "settings.json")
	if got := FolderPath(dir); got != current {
		t.Fatalf("new folder uses %q", got)
	}
	if err := WriteFile(legacy, []byte(`{"wordWrap":true,"other":1}`)); err != nil {
		t.Fatal(err)
	}
	f := LoadFolder(dir)
	if f.Path() != legacy || string(f.Raw("wordWrap")) != "true" {
		t.Fatalf("legacy preferences not loaded: %s, %s", f.Path(), f.Raw("wordWrap"))
	}
	if err := f.Set("wordWrap", json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var prefs map[string]any
	if err := json.Unmarshal(data, &prefs); err != nil || prefs["wordWrap"] != false || prefs["other"] != float64(1) {
		t.Fatalf("legacy write lost preferences: %s (%v)", data, err)
	}
	if err := WriteFile(current, []byte(`{"wordWrap":true}`)); err != nil {
		t.Fatal(err)
	}
	if !f.Reload() || f.Path() != current || string(f.Raw("wordWrap")) != "true" {
		t.Fatalf("FiveLeaf preferences did not take precedence: %s", f.Path())
	}
}

func TestWatchFolderPathSwitch(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".mayoi", "settings.json")
	current := filepath.Join(dir, ".fiveleaf", "settings.json")
	if err := WriteFile(legacy, []byte(`{"wordWrap":true}`)); err != nil {
		t.Fatal(err)
	}
	f := LoadFolder(dir)
	changed := make(chan struct{}, 8)
	stop, err := f.Watch(func() { changed <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	wait := func(path, value string) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case <-changed:
				if f.Path() == path && string(f.Raw("wordWrap")) == value {
					return
				}
			case <-deadline:
				t.Fatalf("did not switch to %s: %s, %s", path, f.Path(), f.Raw("wordWrap"))
			}
		}
	}
	if err := WriteFile(current, []byte(`{"wordWrap":false}`)); err != nil {
		t.Fatal(err)
	}
	wait(current, "false")
	if err := os.Remove(current); err != nil {
		t.Fatal(err)
	}
	wait(legacy, "true")
	if err := WriteFile(current, []byte(`{"wordWrap":false,"other":1}`)); err != nil {
		t.Fatal(err)
	}
	// Set must choose the new file even before its filesystem event is handled.
	if err := f.Set("autoSave", json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	var prefs map[string]any
	if err := json.Unmarshal(data, &prefs); err != nil || prefs["other"] != float64(1) || prefs["wordWrap"] != false || prefs["autoSave"] != false {
		t.Fatalf("write after a path switch lost preferences: %s (%v)", data, err)
	}
}

func TestFolderPathSwitchKeepsInvalidFile(t *testing.T) {
	for _, reloadFirst := range []bool{false, true} {
		name := "write-first"
		if reloadFirst {
			name = "reload-first"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			legacy := filepath.Join(dir, ".mayoi", "settings.json")
			current := filepath.Join(dir, ".fiveleaf", "settings.json")
			original := `{"wordWrap":true,"legacyOnly":1}`
			if err := WriteFile(legacy, []byte(original)); err != nil {
				t.Fatal(err)
			}
			f := LoadFolder(dir)
			incomplete := `{"wordWrap":false,"newOnly":`
			if err := WriteFile(current, []byte(incomplete)); err != nil {
				t.Fatal(err)
			}
			if reloadFirst && f.Reload() {
				t.Fatal("an incomplete file replaced the loaded preferences")
			}
			if err := f.Set("autoSave", json.RawMessage(`false`)); err == nil {
				t.Fatal("wrote cached preferences over an incomplete file")
			}
			for path, want := range map[string]string{legacy: original, current: incomplete} {
				if data, err := os.ReadFile(path); err != nil || string(data) != want {
					t.Fatalf("changed %s: %s (%v)", path, data, err)
				}
			}
			if f.Path() != legacy || string(f.Raw("wordWrap")) != "true" {
				t.Fatal("lost the last valid preferences")
			}
			if err := WriteFile(current, []byte(`{"wordWrap":false,"newOnly":2}`)); err != nil {
				t.Fatal(err)
			}
			if err := f.Set("autoSave", json.RawMessage(`false`)); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(current)
			if err != nil {
				t.Fatal(err)
			}
			var prefs map[string]any
			if err := json.Unmarshal(data, &prefs); err != nil || prefs["newOnly"] != float64(2) || prefs["legacyOnly"] != nil || prefs["autoSave"] != false {
				t.Fatalf("did not use the completed new file: %s (%v)", data, err)
			}
		})
	}
}
