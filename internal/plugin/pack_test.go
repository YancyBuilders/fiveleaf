package plugin

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestPackRepeatedKeepsDependencies(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "", map[string]string{
		"plugin.json":                      `{"name":"packed","main":"main.js"}`,
		"main.js":                          `import { value } from "dependency"; export { value };`,
		"node_modules/dependency/index.js": `export const value = "kept";`,
	})
	if _, err := Pack(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	_, err := Pack(dir)
	if err != nil {
		t.Errorf("packing an already packed plugin: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "vendor", "dependency", "index.js"))
	if err != nil || string(data) != `export const value = "kept";` {
		t.Errorf("packed dependency = %q, %v", data, err)
	}
	assertNoPackTemps(t, dir)
}

func TestPackFailureKeepsFiles(t *testing.T) {
	for _, failure := range []string{"missing", "syntax"} {
		t.Run(failure, func(t *testing.T) {
			dir := packedFixture(t)
			writePlugin(t, dir, "", map[string]string{
				"main.js":  `import { value } from "dependency"; import "./child.js"; import "./broken.js"; export { value };`,
				"child.js": `import { value } from "new-dependency"; export { value };`,
			})
			if failure == "syntax" {
				writePlugin(t, dir, "", map[string]string{"broken.js": `export const broken = ;`})
			}
			before := packFiles(t, dir)
			if _, err := Pack(dir); err == nil || !strings.Contains(err.Error(), "broken.js") {
				t.Fatalf("pack = %v; want an error for broken.js", err)
			}
			if got := packFiles(t, dir); !reflect.DeepEqual(got, before) {
				t.Errorf("failed pack changed files:\nbefore %v\nafter %v", before, got)
			}
			assertNoPackTemps(t, dir)
		})
	}
}

func TestPackWriteFailureRestoresFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions are required")
	}
	dir := packedFixture(t)
	writePlugin(t, dir, "", map[string]string{
		"main.js":  `import { value } from "dependency"; import "./child.js"; import { value as added } from "new-dependency"; export { value, added };`,
		"child.js": `import { value } from "new-dependency"; export { value };`,
	})
	main := filepath.Join(dir, "main.js")
	if err := os.Chmod(main, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(main, 0o600) })
	if f, err := os.OpenFile(main, os.O_WRONLY, 0); err == nil {
		f.Close()
		t.Skip("this user can write read-only files")
	}
	before := packFiles(t, dir)
	if _, err := Pack(dir); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("pack = %v; want a permission error", err)
	}
	if got := packFiles(t, dir); !reflect.DeepEqual(got, before) {
		t.Errorf("failed pack changed files:\nbefore %v\nafter %v", before, got)
	}
	assertNoPackTemps(t, dir)
}

func TestPackRemovesUnusedDependencies(t *testing.T) {
	dir := packedFixture(t)
	writePlugin(t, dir, "", map[string]string{
		"main.js": `import { value } from "new-dependency"; export { value };`,
	})
	if _, err := Pack(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor", "dependency")); !os.IsNotExist(err) {
		t.Errorf("unused dependency remains: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "vendor", "new-dependency", "index.js"))
	if err != nil || string(data) != `export const value = "new";` {
		t.Errorf("new dependency = %q, %v", data, err)
	}
	assertNoPackTemps(t, dir)
}

func TestPackCleanupFailureWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions are required")
	}
	dir := packedFixture(t)
	t.Cleanup(func() {
		filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				os.Chmod(file, 0o755)
			}
			return nil
		})
	})
	old := filepath.Join(dir, "vendor", "dependency")
	if err := os.Chmod(old, 0o500); err != nil {
		t.Fatal(err)
	}
	if f, err := os.CreateTemp(old, ".write-check-"); err == nil {
		f.Close()
		os.Remove(f.Name())
		t.Skip("this user can write read-only directories")
	} else if !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	writePlugin(t, dir, "", map[string]string{
		"main.js": `import { value } from "new-dependency"; export { value };`,
	})
	res, err := Pack(dir)
	if err != nil {
		t.Fatalf("pack completed but returned an error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "vendor", "new-dependency", "index.js"))
	if err != nil || string(data) != `export const value = "new";` {
		t.Errorf("new dependency = %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "main.js"))
	if err != nil || !strings.Contains(string(data), "./vendor/new-dependency/index.js") {
		t.Errorf("packed source = %q, %v", data, err)
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".pack-*"))
	if err != nil || len(leftovers) != 1 {
		t.Fatalf("retained backup folders = %v, %v; want one", leftovers, err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], leftovers[0]) {
		t.Errorf("warnings = %q; want the retained backup path %q", res.Warnings, leftovers[0])
	}
	data, err = os.ReadFile(filepath.Join(leftovers[0], "previous", "dependency", "index.js"))
	if err != nil || string(data) != `export const value = "old";` {
		t.Errorf("retained dependency = %q, %v", data, err)
	}
}

func packedFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "", map[string]string{
		"plugin.json":                          `{"name":"packed","main":"main.js"}`,
		"main.js":                              `import { value } from "dependency"; export { value };`,
		"node_modules/dependency/index.js":     `export const value = "old";`,
		"node_modules/new-dependency/index.js": `export const value = "new";`,
	})
	if _, err := Pack(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func packFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func assertNoPackTemps(t *testing.T, dir string) {
	t.Helper()
	if temps, err := filepath.Glob(filepath.Join(dir, ".pack-*")); err != nil || len(temps) != 0 {
		t.Errorf("temporary pack folders = %v, %v", temps, err)
	}
}
