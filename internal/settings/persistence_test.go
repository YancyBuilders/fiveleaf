package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUserSettingsFlushKeepsPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix file permissions")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"fontSize":15}`), 0o600); err != nil {
		t.Fatal(err)
	}
	prefs := Load(path)
	prefs.FontSize = 18
	prefs.Flush()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || Load(path).FontSize != 18 {
		t.Fatalf("saved settings have mode %03o and font size %v", info.Mode().Perm(), Load(path).FontSize)
	}
}

func TestFolderFailedWriteKeepsValues(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires directory write permissions enforced for a non-root user")
	}
	dir := t.TempDir()
	path := FolderPath(dir)
	original := []byte(`{"wordWrap":false}`)
	if err := WriteFile(path, original); err != nil {
		t.Fatal(err)
	}
	f := LoadFolder(dir)
	gen := f.Gen()
	parent := filepath.Dir(path)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })
	// Reload can still read the current preferences, but the replacement
	// file cannot be created in the read-only directory.
	if err := f.Set("wordWrap", json.RawMessage(`true`)); err == nil {
		t.Fatal("saving in a read-only directory did not fail")
	}
	if got := string(f.Raw("wordWrap")); got != "false" || f.Gen() != gen {
		t.Fatalf("failed write changed the preferences: wordWrap=%s, gen=%d, want false and %d", got, f.Gen(), gen)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != string(original) {
		t.Fatalf("failed write changed the file: %s, err=%v", raw, err)
	}
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.Set("wordWrap", json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if got := string(f.Raw("wordWrap")); got != "true" || f.Gen() != gen+1 {
		t.Fatalf("successful retry did not publish the preferences: wordWrap=%s, gen=%d", got, f.Gen())
	}
	if got := string(LoadFolder(dir).Raw("wordWrap")); got != "true" {
		t.Fatalf("successful retry was not saved: wordWrap=%s", got)
	}
}

func TestFlushRetriesFailedWrites(t *testing.T) {
	for _, name := range []string{"settings.json", "state.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")
			s := Load(path)
			s.FontSize = 19
			s.Recent = []string{"/kept"}
			written := 0
			s.OnPrefsWritten(func() { written++ })

			// A failed replacement must not count as saved. Once the target
			// is writable again, flushing unchanged values should retry it.
			blocked := filepath.Join(dir, name)
			if err := os.Mkdir(blocked, 0o755); err != nil {
				t.Fatal(err)
			}
			s.Flush()
			if name == "settings.json" && written != 0 {
				t.Errorf("failed preference write reported as saved %d times", written)
			}
			if err := os.Remove(blocked); err != nil {
				t.Fatal(err)
			}
			s.Flush()
			if written != 1 {
				t.Errorf("successful preference writes = %d, want 1", written)
			}
			data, err := os.ReadFile(blocked)
			if err != nil {
				t.Fatalf("unchanged values were not saved after recovery: %v", err)
			}
			if name == "settings.json" {
				var p Prefs
				if err := json.Unmarshal(data, &p); err != nil || p.FontSize != 19 {
					t.Fatalf("recovered preferences: fontSize=%v, err=%v", p.FontSize, err)
				}
			} else {
				var state State
				if err := json.Unmarshal(data, &state); err != nil || len(state.Recent) != 1 || state.Recent[0] != "/kept" {
					t.Fatalf("recovered state: recent=%q, err=%v", state.Recent, err)
				}
			}
		})
	}
}

func TestLoadIgnoresNullWindowsAndSessions(t *testing.T) {
	for _, name := range []string{"settings.json", "state.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")
			data := []byte(`{"windows":[null,{"id":7,"folder":"/kept"},null],"sessions":{"/empty":null,"/kept":{"columns":[{"tabs":[{"file":"/kept/a.go"}]}]}}}`)
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
			s := Load(path)
			if len(s.Windows) != 1 || s.Windows[0] == nil || s.Windows[0].ID != 7 || s.Windows[0].Folder != "/kept" {
				t.Fatalf("valid windows were not preserved: %+v", s.Windows)
			}
			if len(s.Sessions) != 1 || s.Sessions["/kept"] == nil || len(s.Sessions["/kept"].Columns) != 1 {
				t.Fatalf("valid sessions were not preserved: %+v", s.Sessions)
			}
			if s.Windows[0].Layout != DefaultLayout || s.Sessions["/kept"].Layout != DefaultLayout {
				t.Fatal("valid entries did not receive the default layout")
			}
			s.NewWindow()
			s.Flush()
			if got := Load(path); len(got.Windows) != 2 || len(got.Sessions) != 1 {
				t.Fatalf("normalized state did not round trip: %+v", got.State)
			}
		})
	}
}
