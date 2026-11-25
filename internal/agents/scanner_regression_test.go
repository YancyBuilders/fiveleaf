package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCodexUsesOuterMetadataAndSkipsInternalSessions(t *testing.T) {
	home := t.TempDir()
	for _, fixture := range []struct {
		name, source, originator string
		visible                  bool
	}{
		{"main", `"vscode"`, "Codex Desktop", true},
		{"exec", `"exec"`, "codex_cli_rs", false},
		{"origin-exec", `"cli"`, "codex_exec", false},
		{"worker", `{"subagent":{"thread_spawn":{"parent_thread_id":"main"}}}`, "Codex Desktop", false},
		{"review", `{"subagent":"review"}`, "Codex Desktop", false},
		{"legacy-worker", `"subagent"`, "Codex Desktop", false},
	} {
		path := filepath.Join(home, "rollout-"+fixture.name+".jsonl")
		write(t, path, `{"type":"session_meta","payload":{"id":"`+fixture.name+`","cwd":"/workspace","source":`+fixture.source+`,"originator":"`+fixture.originator+`"}}
{"type":"session_meta","payload":{"id":"inherited","cwd":"/other","source":"cli"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /workspace\nRules"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>context</environment_context>"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# Files mentioned by the user:\nimage.png\n\n## My request:\nFix the session list"}]}}
`)
		session, visible := readCodex(path)
		if visible != fixture.visible {
			t.Errorf("%s visible=%t, want %t", fixture.name, visible, fixture.visible)
		}
		if visible && (session.ID != fixture.name || session.Dir != "/workspace" || session.Prompt != "Fix the session list") {
			t.Errorf("outer session was replaced or prompt polluted: %+v", session)
		}
	}
}

func TestCodexEventPromptAndMalformedHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-session.jsonl")
	write(t, path, `{"type":"session_meta","payload":{"id":"session","cwd":"/workspace","source":"cli"}}
{"type":"event_msg","payload":{"type":"user_message","message":"Fix navigation"}}
`)
	session, visible := readCodex(path)
	if !visible || session.Prompt != "Fix navigation" {
		t.Fatalf("event prompt: %+v, %t", session, visible)
	}
	write(t, path, `{"type":"session_meta","payload":{"cwd":"/workspace","source":"cli"}}
{"type":"session_meta","payload":{"id":"inherited","cwd":"/other","source":"cli"}}
`)
	if session, visible = readCodex(path); visible {
		t.Fatalf("malformed outer header became inherited session: %+v", session)
	}
}

func TestScanDeduplicatesIdentityAndKeepsDistinctTitles(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".codex", "sessions", "2026", "11", "17")
	older := filepath.Join(base, "rollout-old.jsonl")
	newer := filepath.Join(base, "rollout-new.jsonl")
	other := filepath.Join(base, "rollout-other.jsonl")
	write(t, older, `{"type":"session_meta","payload":{"id":"same","cwd":"/workspace","source":"cli"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":"Original task"}}
`)
	write(t, newer, `{"type":"session_meta","payload":{"id":"same","cwd":"/workspace","source":"cli"}}`+"\n")
	write(t, other, `{"type":"session_meta","payload":{"id":"different","cwd":"/workspace","source":"cli"}}`+"\n")
	write(t, filepath.Join(home, ".codex", "session_index.jsonl"), `{"id":"same","thread_name":"Shared title"}
{"id":"different","thread_name":"Shared title"}
`)
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner()
	for range 3 {
		rows := scanner.Scan(&Env{Home: home})
		if len(rows) != 2 {
			t.Fatalf("rows=%+v, want two distinct identities", rows)
		}
		row := byID(rows)["same"]
		if row.Transcript != newer || row.Prompt != "Original task" || row.Title != "Shared title" || !row.Updated.After(oldTime) {
			t.Fatalf("duplicate merge lost latest transcript or metadata: %+v", row)
		}
	}
}

func TestScanDeduplicatesPluginListsByAgent(t *testing.T) {
	t.Cleanup(func() { Use(nil) })
	stamp := time.Now()
	list := func(context.Context) ([]Session, error) {
		return []Session{
			{ID: "shared", Title: "old", Dir: "/workspace", Updated: stamp},
			{ID: "shared", Title: "new", Updated: stamp.Add(time.Minute)},
		}, nil
	}
	Use([]Agent{{ID: "first", List: list}, {ID: "second", List: list}})
	rows := NewScanner().Scan(&Env{Home: t.TempDir()})
	if len(rows) != 2 || rows[0].Harness == rows[1].Harness {
		t.Fatalf("independent agents merged: %+v", rows)
	}
	for _, row := range rows {
		if row.Title != "new" || row.Dir != "/workspace" {
			t.Fatalf("list metadata merge: %+v", row)
		}
	}
}

func TestClaudeRootAliasesAndWorkspaceAliases(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".claude")
	write(t, filepath.Join(root, "projects", "project", "session.jsonl"), `{"type":"user","cwd":"/workspace","message":{"content":"Task"}}`+"\n")
	alias := filepath.Join(home, ".claude-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skip(err)
	}
	env := &Env{Home: home, ConfigDir: alias}
	if roots := env.ClaudeRoots(); len(roots) != 1 || roots[0] != root {
		t.Fatalf("config aliases repeated: %v", roots)
	}
	rows := NewScanner().Scan(env)
	if len(rows) != 1 {
		t.Fatalf("session duplicated by config symlink: %+v", rows)
	}
	directory := filepath.Join(home, "workspace")
	if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceAlias := filepath.Join(home, "workspace-link")
	if err := os.Symlink(directory, workspaceAlias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, filepath.Join(directory, "nested"), workspaceAlias} {
		if !(Session{Dir: path}).In(workspaceAlias) {
			t.Errorf("workspace alias omitted %s", path)
		}
	}
	if (Session{Dir: directory + "-sibling"}).In(directory) {
		t.Fatal("sibling prefix included in workspace")
	}
	if !(Session{Dir: directory}).In(string(filepath.Separator)) {
		t.Fatal("filesystem root excluded its descendants")
	}
}

func TestCustomCodexAndPiHomes(t *testing.T) {
	home := t.TempDir()
	codexRoot, piRoot := filepath.Join(home, "codex profile"), filepath.Join(home, "pi profile")
	write(t, filepath.Join(codexRoot, "sessions", "rollout-codex.jsonl"), `{"type":"session_meta","payload":{"id":"codex-id","cwd":"/workspace","source":"cli"}}`+"\n")
	write(t, filepath.Join(codexRoot, "session_index.jsonl"), `{"id":"codex-id","thread_name":"Codex title"}`+"\n")
	piPath := filepath.Join(piRoot, "sessions", "--workspace--", "date_pi-id.jsonl")
	write(t, piPath, `{"type":"session","version":3,"id":"pi-id","cwd":"/workspace"}
{"type":"message","message":{"role":"user","content":[{"type":"text","text":"Pi task"}]}}
{"type":"session_info","name":"Pi title"}
`)
	env := &Env{Home: home, CodexHome: "~/codex profile", PiDir: "~/pi profile"}
	rows := NewScanner().Scan(env)
	if len(rows) != 2 {
		t.Fatalf("custom profile sessions: %+v", rows)
	}
	indexed := byID(rows)
	codex, pi := indexed["codex-id"], indexed["pi-id"]
	if codex.Root != codexRoot || codex.Title != "Codex title" || pi.Root != piRoot || pi.Harness != Pi || pi.Prompt != "Pi task" || pi.Title != "Pi title" {
		t.Fatalf("profile metadata: %+v", rows)
	}
	if command := env.ResumeCommand(codex); command != "CODEX_HOME="+quote(codexRoot)+" codex resume codex-id" {
		t.Errorf("Codex profile resume: %s", command)
	}
	if command := env.ResumeCommand(pi); command != "PI_CODING_AGENT_DIR="+quote(piRoot)+" pi --session "+quote(piPath) {
		t.Errorf("Pi profile resume: %s", command)
	}
}

func TestPiReadsLateTitleAndIgnoresEmptySessions(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".pi", "agent", "sessions", "--workspace--", "session.jsonl")
	write(t, path, `{"type":"session","version":3,"id":"pi-id","cwd":"/workspace"}
{"type":"message","message":{"role":"user","content":"Pi task"}}
`+strings.Repeat("{}\n", headMax/2)+`{"type":"session_info","name":"Renamed"}`+"\n")
	write(t, filepath.Join(filepath.Dir(path), "empty.jsonl"), `{"type":"session","version":3,"id":"empty","cwd":"/workspace"}`+"\n")
	rows := NewScanner().Scan(&Env{Home: home})
	if len(rows) != 1 || rows[0].Title != "Renamed" || rows[0].Root != "" {
		t.Fatalf("Pi rows: %+v", rows)
	}
	if command := (&Env{Home: home}).ResumeCommand(rows[0]); command != "pi --session "+quote(path) {
		t.Errorf("Pi resume: %s", command)
	}
}

func TestScanRefreshesCachedPathsWhenConfigAliasChanges(t *testing.T) {
	for _, derived := range []bool{false, true} {
		name := "file"
		if derived {
			name = "derived-transcript"
		}
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { Use(nil) })
			home := t.TempDir()
			root, alias := filepath.Join(home, "profile"), filepath.Join(home, "alias")
			write(t, filepath.Join(root, "metadata.jsonl"), `{"type":"user","cwd":"/workspace","message":{"content":"Task"}}`+"\n")
			write(t, filepath.Join(root, "chat.jsonl"), "{}\n")
			if err := os.Symlink(root, alias); err != nil {
				t.Skip(err)
			}
			var reads, transcriptReads atomic.Int32
			agent := Agent{
				ID: "cache-alias",
				Files: func(env *Env) []File {
					return []File{{Path: filepath.Join(env.ConfigDir, "metadata.jsonl"), Root: env.ConfigDir}}
				},
				Read: func(path string) (Session, bool) {
					reads.Add(1)
					return readTranscript(path, Claude)
				},
			}
			transcriptName := "metadata.jsonl"
			if derived {
				transcriptName = "chat.jsonl"
				agent.Transcript = func(path string) string {
					transcriptReads.Add(1)
					return filepath.Join(filepath.Dir(path), transcriptName)
				}
			}
			Use([]Agent{agent})
			scanner := NewScanner()
			env := &Env{Home: home, ConfigDir: alias}
			initial := scanner.Scan(env)
			if len(initial) != 1 || initial[0].Root != alias || initial[0].Transcript != filepath.Join(alias, transcriptName) {
				t.Fatalf("initial alias paths: %+v", initial)
			}
			env.ConfigDir = root
			current := scanner.Scan(env)
			if len(current) != 1 || current[0].Key() != initial[0].Key() || current[0].Root != root || current[0].Transcript != filepath.Join(root, transcriptName) {
				t.Fatalf("cache kept previous alias paths: %+v", current)
			}
			if err := os.Remove(alias); err != nil {
				t.Fatal(err)
			}
			current = scanner.Scan(env)
			if len(current) != 1 || current[0].Root != root || current[0].Transcript != filepath.Join(root, transcriptName) || !current[0].Updated.Equal(initial[0].Updated) {
				t.Fatalf("cached paths changed after alias removal: %+v", current)
			}
			if _, err := os.Stat(current[0].Transcript); err != nil {
				t.Fatalf("cached transcript is not accessible: %v", err)
			}
			if reads.Load() != 1 || derived && transcriptReads.Load() != 2 {
				t.Errorf("cached metadata reread: file=%d, transcript=%d", reads.Load(), transcriptReads.Load())
			}
		})
	}
}

func TestScanKeepsSameIDInDistinctProfilesAndEnvironments(t *testing.T) {
	home := t.TempDir()
	stamp := time.Now().Add(-time.Hour)
	for _, name := range []string{".claude", ".claude-work"} {
		path := filepath.Join(home, name, "projects", "project", "shared.jsonl")
		write(t, path, `{"type":"user","cwd":"/workspace","message":{"content":"`+name+`"}}`+"\n")
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	scanner := NewScanner()
	rows := scanner.Scan(&Env{Home: home})
	if len(rows) != 2 || rows[0].ID != rows[1].ID || rows[0].Key() == rows[1].Key() || rows[0].Root == rows[1].Root {
		t.Fatalf("independent profiles collapsed: %+v", rows)
	}
	other := t.TempDir()
	path := filepath.Join(other, ".claude", "projects", "project", "shared.jsonl")
	write(t, path, `{"type":"user","cwd":"/workspace","message":{"content":"another"}}`+"\n")
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	rows = scanner.Scan(&Env{Home: other})
	if len(rows) != 1 || rows[0].Prompt != "another" || rows[0].Root != filepath.Join(other, ".claude") {
		t.Fatalf("prior environment metadata leaked: %+v", rows)
	}
}
