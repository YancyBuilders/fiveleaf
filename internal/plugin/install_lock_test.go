package plugin

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginChangeLockAfterCrash(t *testing.T) {
	const holderDir = "FIVELEAF_TEST_PLUGIN_LOCK_DIR"
	if dir := os.Getenv(holderDir); dir != "" {
		err := changePlugins(dir, func(work string, _ Records) error {
			if err := os.Rename(filepath.Join(dir, "paper"), filepath.Join(work, "previous")); err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, work)
			_, err := io.Copy(io.Discard, os.Stdin)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return
	}

	dir := t.TempDir()
	writePlugin(t, dir, "paper", map[string]string{ManifestFile: `{"name":"paper","version":"1.0.0"}`})
	repo := gitRepo(t, map[string]string{ManifestFile: `{"name":"paper","version":"2.0.0"}`})
	holder := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPluginChangeLockAfterCrash$", "-test.timeout=30s")
	holder.Env = append(os.Environ(), holderDir+"="+dir)
	holder.Stderr = os.Stderr
	if _, err := holder.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	ready, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if holder.ProcessState == nil {
			holder.Process.Kill()
			holder.Wait()
		}
	}()
	work, err := bufio.NewReader(ready).ReadString('\n')
	if err != nil {
		t.Fatalf("waiting for transaction holder: %v", err)
	}
	if _, err := Install(t.Context(), dir, repo, ""); err == nil || !strings.Contains(err.Error(), "locking plugins for a change") {
		t.Fatalf("install while another process holds the lock: %v", err)
	}
	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	var killed *exec.ExitError
	if err := holder.Wait(); !errors.As(err, &killed) {
		t.Fatalf("waiting for killed transaction holder: %v", err)
	}
	if _, err := Install(t.Context(), dir, repo, ""); err != nil {
		t.Fatalf("install after holder was killed: %v", err)
	}
	if manifest, err := ReadManifest(filepath.Join(dir, "paper")); err != nil || manifest.Version != "2.0.0" {
		t.Fatalf("reinstalled plugin: %+v, %v", manifest, err)
	}
	if source := ReadRecords(dir)["paper"]; source.URL != repo {
		t.Fatalf("reinstalled source = %+v", source)
	}
	if manifest, err := ReadManifest(filepath.Join(strings.TrimSpace(work), "previous")); err != nil || manifest.Version != "1.0.0" {
		t.Fatalf("interrupted transaction backup: %+v, %v", manifest, err)
	}
}

func TestPluginChangeLockPreservesBackups(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".plugins.lock", "previous")
	writePlugin(t, legacy, "", map[string]string{"version": "legacy"})
	var backup string
	failure := errors.New("rollback failed")
	err := changePlugins(dir, func(work string, _ Records) error {
		backup = filepath.Join(work, "previous")
		writePlugin(t, backup, "", map[string]string{"version": "original"})
		return failure
	})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), backup) {
		t.Fatalf("expected backup path %q in rollback error, got %v", backup, err)
	}
	lockPath := filepath.Join(dir, ".plugins-write.lock")
	lock, err := os.Open(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	before, err := lock.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteRecords(dir, nil); err != nil {
		t.Fatal(err)
	}
	if after, err := os.Stat(lockPath); err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		t.Fatalf("lock file replaced across transactions: %v, %v", after, err)
	}
	for path, want := range map[string]string{legacy: "legacy", backup: "original"} {
		if got, err := os.ReadFile(filepath.Join(path, "version")); err != nil || string(got) != want {
			t.Errorf("backup at %s = %q, %v", path, got, err)
		}
	}
}
