package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryTrashPathNativeRecords(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude/projects/project/claude.jsonl")
	qoder := filepath.Join(home, ".qoder/projects/project/qoder.jsonl")
	codex := filepath.Join(home, ".codex/sessions/2026/11/25/rollout-codex.jsonl")
	pi := filepath.Join(home, ".pi/agent/sessions/project/pi.jsonl")
	grok := filepath.Join(home, ".grok/sessions/project/grok")
	index := filepath.Join(home, ".codex/session_index.jsonl")
	write(t, claude, `{"type":"ai-title","aiTitle":"Claude history"}`)
	write(t, qoder, `{"type":"ai-title","aiTitle":"Qoder history"}`)
	write(t, codex, `{"type":"session_meta","payload":{"id":"codex","cwd":"/project","source":"cli"}}`)
	write(t, pi, `{"type":"session","id":"pi","cwd":"/project"}`+"\n"+
		`{"type":"message","message":{"role":"user","content":"Pi history"}}`)
	write(t, filepath.Join(grok, "summary.json"), `{"info":{"id":"grok","cwd":"/project"},"generated_title":"Grok history"}`)
	write(t, filepath.Join(grok, "chat_history.jsonl"), `{}`)
	write(t, index, `{"id":"codex","thread_name":"Shared index"}`)
	write(t, filepath.Join(filepath.Dir(claude), "neighbor.jsonl"), `{"type":"ai-title","aiTitle":"Neighbor"}`)

	scanner := NewScanner()
	env := &Env{Home: home}
	sessions := byID(scanner.Scan(env))
	trash := t.TempDir()
	for id, path := range map[string]string{"claude": claude, "qoder": qoder, "codex": codex, "pi": pi, "grok": grok} {
		session := sessions[id]
		if !session.CanTrashHistory() {
			t.Fatalf("%s: no Trash path in %+v", id, session)
		}
		got, err := session.HistoryTrashPath()
		if err != nil || got != canonicalPath(path) {
			t.Fatalf("%s: Trash path = %q, %v, want %q", id, got, err, canonicalPath(path))
		}
		if err := os.Rename(got, filepath.Join(trash, id)); err != nil {
			t.Fatal(err)
		}
	}
	if rows := scanner.Scan(env); len(rows) != 1 || rows[0].ID != "neighbor" {
		t.Fatalf("remaining history = %+v", rows)
	}
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("shared Codex index changed: %v", err)
	}
}

func TestHistoryTrashPathRejectsReplacedFile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude/projects/project/old.jsonl")
	const original = `{"type":"ai-title","aiTitle":"Archived"}`
	const replacement = `{"type":"ai-title","aiTitle":"Replaced"}`
	write(t, path, original)
	env := &Env{Home: home}
	scanner := NewScanner()
	old := byID(scanner.Scan(env))["old"]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if _, err := old.HistoryTrashPath(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file accepted: %v", err)
	}
	write(t, path, replacement)
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := old.HistoryTrashPath(); !errors.Is(err, ErrHistoryChanged) {
		t.Fatalf("replacement file accepted: %v", err)
	}
	updated := byID(scanner.Scan(env))["old"]
	if _, err := updated.HistoryTrashPath(); err != nil || updated.Title != "Replaced" {
		t.Fatalf("replacement not refreshed: %+v, %v", updated, err)
	}
}

func TestHistoryTrashPathRejectsAmbiguousSources(t *testing.T) {
	home := t.TempDir()
	const rollout = `{"type":"session_meta","payload":{"id":"duplicate","cwd":"/project","source":"cli"}}`
	write(t, filepath.Join(home, ".codex/sessions/rollout-one.jsonl"), rollout)
	write(t, filepath.Join(home, ".codex/sessions/rollout-two.jsonl"), rollout)
	pluginPath := filepath.Join(home, "own-project.json")
	write(t, pluginPath, `{"id":"plugin-file"}`)
	Use([]Agent{
		WithPattern(Agent{ID: "plugin-files"}, Pattern{Glob: pluginPath, ID: "id"}),
		{ID: "plugin-list", List: func(context.Context) ([]Session, error) {
			return []Session{{ID: "plugin-row", Transcript: pluginPath}}, nil
		}},
	})
	t.Cleanup(func() { Use(nil) })
	sessions := byID(NewScanner().Scan(&Env{Home: home}))
	for _, id := range []string{"duplicate", "plugin-file", "plugin-row"} {
		session := sessions[id]
		if session.ID == "" || session.CanTrashHistory() {
			t.Fatalf("%s: got %+v, want visible history without a Trash path", id, session)
		}
	}
}

func TestHistoryTrashPathRejectsLinkedFiles(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()
	actual := filepath.Join(profile, "projects/project/actual.jsonl")
	write(t, actual, `{"type":"ai-title","aiTitle":"Actual"}`)
	if err := os.Symlink(profile, filepath.Join(home, ".claude")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	other := filepath.Join(t.TempDir(), "outside.jsonl")
	write(t, other, `{"type":"ai-title","aiTitle":"Outside"}`)
	if err := os.Symlink(other, filepath.Join(filepath.Dir(actual), "link.jsonl")); err != nil {
		t.Fatal(err)
	}
	sessions := byID(NewScanner().Scan(&Env{Home: home}))
	if got, err := sessions["actual"].HistoryTrashPath(); err != nil || got != canonicalPath(actual) {
		t.Fatalf("linked profile was not supported: %q, %v", got, err)
	}
	if linked := sessions["link"]; linked.ID == "" || linked.CanTrashHistory() {
		t.Fatalf("linked transcript offered for trash: %+v", linked)
	}
}

func TestHistoryTrashPathRequiresDedicatedGrokDirectory(t *testing.T) {
	home := t.TempDir()
	group := filepath.Join(home, ".grok/sessions/project/chat")
	transcript := filepath.Join(group, "chat_history.jsonl")
	write(t, filepath.Join(group, "summary.json"), `{"info":{"id":"chat","cwd":"/project"},"generated_title":"Chat"}`)
	env := &Env{Home: home}
	scanner := NewScanner()
	before := byID(scanner.Scan(env))["chat"]
	write(t, transcript, `{}`)
	if _, err := before.HistoryTrashPath(); !errors.Is(err, ErrHistoryChanged) {
		t.Fatalf("changed session directory accepted: %v", err)
	}
	refreshed := byID(scanner.Scan(env))["chat"]
	if found, err := refreshed.HistoryTrashPath(); err != nil || found != canonicalPath(group) {
		t.Fatalf("refreshed session: %q, %v", found, err)
	}
	write(t, filepath.Join(group, "user-notes.md"), "Notes")
	if _, err := refreshed.HistoryTrashPath(); !errors.Is(err, ErrHistoryChanged) {
		t.Fatalf("directory with unrelated files accepted: %v", err)
	}
	if current := byID(scanner.Scan(env))["chat"]; current.ID == "" || current.CanTrashHistory() {
		t.Fatalf("directory with unrelated files offered for trash: %+v", current)
	}
}
