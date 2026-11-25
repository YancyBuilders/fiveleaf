package vfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileFollowsSymlink(t *testing.T) {
	for _, kind := range []string{"absolute", "relative", "chain"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target.txt")
			if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "alias.txt")
			to := target
			if kind == "relative" {
				to = filepath.Base(target)
			} else if kind == "chain" {
				to = filepath.Join(dir, "second.txt")
				if err := os.Symlink(filepath.Base(target), to); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			if err := os.Symlink(to, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			info, err := WriteFile(link, []byte("after"), 0o644)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := os.Readlink(link); err != nil || got != to {
				t.Errorf("link = %q, %v; want %q", got, err, to)
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "after" {
				t.Errorf("target = %q, %v; want after", data, err)
			}
			if info.Name() != filepath.Base(link) || info.Size() != 5 || info.Mode().Perm() != before.Mode().Perm() {
				t.Errorf("file info = %v; want alias name, saved length and original permissions", info)
			}
			if temps, err := filepath.Glob(filepath.Join(dir, ".*.fiveleaf-*")); err != nil || len(temps) != 0 {
				t.Errorf("temporary files = %v, %v", temps, err)
			}
		})
	}
}

func TestWriteFileKeepsInvalidSymlink(t *testing.T) {
	for _, kind := range []string{"missing", "loop", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			link := filepath.Join(dir, "alias.txt")
			to := "missing.txt"
			if kind == "loop" {
				to = filepath.Base(link)
			} else if kind == "directory" {
				to = "folder"
				if err := os.Mkdir(filepath.Join(dir, to), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(to, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := WriteFile(link, []byte("after"), 0o644); err == nil {
				t.Error("write to an invalid file target succeeded")
			}
			if got, err := os.Readlink(link); err != nil || got != to {
				t.Errorf("link = %q, %v; want %q", got, err, to)
			}
		})
	}
}

func TestWriteFileCreatesFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "new.txt")
	if _, err := WriteFile(file, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "new" {
		t.Fatalf("file = %q, %v; want new", data, err)
	}
}
