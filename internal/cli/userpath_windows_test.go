package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsKeepsOtherFFOnPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	dir := installDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "ff.ps1")
	if err := os.WriteFile(other, []byte("Write-Output other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallOnPath(home, nil, "FiveLeaf.exe"); err == nil || !strings.Contains(err.Error(), other) {
		t.Fatalf("expected a conflict with %s: %v", other, err)
	}
	t.Setenv("PATH", dir)
	if err := RemoveFromUserPath(home); err != nil || os.Getenv("PATH") != dir {
		t.Fatalf("removed the PowerShell command's directory from PATH: %v", err)
	}
}

func TestEditPath(t *testing.T) {
	dir := `C:\Users\Me\AppData\Local\FiveLeaf\bin`
	duplicate := `%USERPROFILE%\bin;` + strings.ToUpper(dir)
	for _, tt := range []struct {
		name, value string
		add         bool
		want        string
	}{
		{"add to empty", "", true, dir},
		{"append without expanding", `%USERPROFILE%\bin;C:\Tools`, true, `%USERPROFILE%\bin;C:\Tools;` + dir},
		{"existing is unchanged", duplicate, true, duplicate},
		{"remove ignores case", `C:\Tools;` + strings.ToUpper(dir) + `;%LOCALAPPDATA%\npm`, false, `C:\Tools;%LOCALAPPDATA%\npm`},
		{"remove when absent keeps quotes", `"C:\A;B";C:\Tools`, false, `"C:\A;B";C:\Tools`},
		{"remove every copy", dir + `;C:\Tools;` + dir, false, `C:\Tools`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := editPath(tt.value, dir, tt.add); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
