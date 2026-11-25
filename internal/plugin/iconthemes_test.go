package plugin

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestIconThemeManifest(t *testing.T) {
	good := IconThemeContrib{ID: "icons.night", Label: "Night Icons", Path: "themes/night.json"}
	parse := func(contributions ...IconThemeContrib) (*Manifest, error) {
		data, err := json.Marshal(Manifest{Name: "icons", Contributes: Contributes{IconThemes: contributions}})
		if err != nil {
			t.Fatal(err)
		}
		return ParseManifest(data)
	}
	m, err := parse(good)
	if err != nil || len(m.Contributes.IconThemes) != 1 || m.Contributes.IconThemes[0] != good {
		t.Fatalf("icon theme manifest = %+v, %v", m, err)
	}
	if m.HasCode() || !m.DefaultOn() {
		t.Fatal("an icon-only plugin should be enabled without running code")
	}
	for _, id := range []string{"", "other.night", "icons."} {
		t.Run("id="+id, func(t *testing.T) {
			bad := good
			bad.ID = id
			if _, err := parse(bad); err == nil {
				t.Fatalf("accepted icon theme id %q", id)
			}
		})
	}
	for _, label := range []string{"", " \t\n"} {
		bad := good
		bad.Label = label
		if _, err := parse(bad); err == nil {
			t.Fatalf("accepted icon theme label %q", label)
		}
	}
	for _, path := range []string{"", ".", "..", "../icons.json", "/icons.json", "themes/../icons.json", "./icons.json", "themes//icons.json", `..\icons.json`, `C:\icons.json`, "C:/icons.json", "https://example.com/icons.json", "icons\x00.json"} {
		t.Run("path="+path, func(t *testing.T) {
			bad := good
			bad.Path = path
			if _, err := parse(bad); err == nil {
				t.Fatalf("accepted icon theme path %q", path)
			}
		})
	}
	if _, err := parse(good, good); err == nil {
		t.Fatal("accepted duplicate icon theme ids")
	}
	other := good
	other.ID = "icons.day"
	if _, err := parse(good, other); err != nil {
		t.Fatalf("two icon themes may share one file: %v", err)
	}
}

func TestInstallIconThemePlugin(t *testing.T) {
	r, _, dirs := newRegistry(t)
	const manifest = `{"name":"icons","contributes":{"iconThemes":[{"id":"icons.night","label":"Night Icons","path":"themes/night.json"}]}}`
	const theme = `{"file":"file","folder":"file","icons":{"file":"icons/file.svg"}}`
	const firstSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path fill="#89b4fa" d="M2 1h8l4 4v10H2z"/></svg>`
	const secondSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path fill="#a6e3a1" d="M2 1h8l4 4v10H2z"/></svg>`
	repo := gitRepo(t, map[string]string{
		"plugin.json":           manifest,
		"themes/night.json":     theme,
		"themes/icons/file.svg": firstSVG,
	})
	m, err := Install(context.Background(), dirs.Plugins, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.HasCode() || len(m.Contributes.IconThemes) != 1 || m.Contributes.IconThemes[0].ID != "icons.night" {
		t.Fatalf("installed manifest = %+v", m)
	}
	check := func(wantSVG string) {
		t.Helper()
		r.Load()
		p := r.Plugin("icons")
		if p == nil || len(r.Enabled()) != 1 {
			t.Fatalf("icon plugin not enabled: %v", r.Enabled())
		}
		r.Activate("onStartup")
		if p.Started() {
			t.Fatal("an icon-only plugin started a runtime")
		}
		for path, want := range map[string]string{"themes/night.json": theme, "themes/icons/file.svg": wantSVG} {
			data, err := fs.ReadFile(p.FS(), path)
			if err != nil || string(data) != want {
				t.Fatalf("installed %s = %q, %v", path, data, err)
			}
		}
	}
	check(firstSVG)
	writePlugin(t, repo, "", map[string]string{"themes/icons/file.svg": secondSVG})
	if _, err := git(context.Background(), repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qam", "Update icon"); err != nil {
		t.Fatal(err)
	}
	if changed, err := Update(context.Background(), dirs.Plugins, "icons"); err != nil || !changed {
		t.Fatalf("icon update = %v, %v", changed, err)
	}
	check(secondSVG)
	if err := Remove(dirs.Plugins, "icons"); err != nil {
		t.Fatal(err)
	}
	r.Load()
	if p := r.Plugin("icons"); p != nil {
		t.Fatalf("removed icon plugin remains: %s", filepath.Join(dirs.Plugins, p.Name))
	}
}
