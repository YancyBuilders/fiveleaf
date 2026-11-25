package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/roundtable"
	"fiveleaf/internal/settings"
)

func TestAgentContextCompactionBudgets(t *testing.T) {
	for _, tc := range []struct {
		name, raw, model string
		known, enabled   bool
		threshold        int
	}{
		{name: "defaults", known: true, enabled: true, threshold: 83616},
		{name: "disabled", raw: `{"enabled":false}`, known: true},
		{name: "reserve", raw: `{"reserveTokens":15000}`, known: true, enabled: true, threshold: 85000},
		{name: "zero reserve", raw: `{"reserveTokens":0}`, known: true, enabled: true, threshold: 100000},
		{name: "large reserve", raw: `{"reserveTokens":200000}`, known: true, enabled: true},
		{name: "per model", raw: `{"modelOverrides":{"provider/nested/model":{"reserveTokens":4096}}}`, model: "provider/nested/model", known: true, enabled: true, threshold: 95904},
		{name: "exact model key", raw: `{"modelOverrides":{"Provider/model":{"reserveTokens":4096}}}`, model: "provider/model", known: true, enabled: true, threshold: 83616},
		{name: "invalid ordinary despite override", raw: `{"reserveTokens":-1,"modelOverrides":{"provider/model":{"reserveTokens":4096}}}`, model: "provider/model"},
		{name: "invalid keep while disabled", raw: `{"enabled":false,"keepRecentTokens":null}`},
		{name: "invalid matching override", raw: `{"modelOverrides":{"provider/model":null}}`, model: "provider/model"},
		{name: "irrelevant invalid override", raw: `{"modelOverrides":{"provider/other":null}}`, model: "provider/model", known: true, enabled: true, threshold: 83616},
		{name: "fractional reserve", raw: `{"reserveTokens":1.5}`},
		{name: "unsafe integer", raw: `{"reserveTokens":9007199254740992}`},
		{name: "invalid enabled", raw: `{"enabled":"false"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &agentChat{currentModel: tc.model, contextUsage: &acp.UsageState{Used: 30000, Size: 100000}, contextPolicy: parseAgentCompactionPolicy(json.RawMessage(tc.raw))}
			got := (&app{}).contextSnapshot(s)
			if got.compactionKnown != tc.known || got.compactionEnabled != tc.enabled || got.compactAt != tc.threshold {
				t.Fatalf("budget known=%v enabled=%v threshold=%d, want %v %v %d", got.compactionKnown, got.compactionEnabled, got.compactAt, tc.known, tc.enabled, tc.threshold)
			}
		})
	}
}

func writeAgentContextFixture(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAgentContextPiSettingsAndTrust(t *testing.T) {
	for _, tc := range []struct {
		name, globalTrust, trust string
		project                  string
		known                    bool
		reserve, modelReserve    int
	}{
		{name: "RPC ask ignores project", known: true, reserve: 10000, modelReserve: 20000},
		{name: "global trust", globalTrust: `"always"`, known: true, reserve: 8000, modelReserve: 20000},
		{name: "saved ancestor trust", trust: "ancestor", known: true, reserve: 8000, modelReserve: 20000},
		{name: "nearest denies", globalTrust: `"always"`, trust: "nearest-deny", known: true, reserve: 10000, modelReserve: 20000},
		{name: "null skips to ancestor", trust: "null-nearest", known: true, reserve: 8000, modelReserve: 20000},
		{name: "malformed trust", trust: "invalid"},
		{name: "malformed trust with empty project", trust: "invalid", project: `{}`},
		{name: "untrusted malformed project", project: `{`, known: true, reserve: 10000, modelReserve: 20000},
		{name: "trusted malformed project", globalTrust: `"always"`, project: `{`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			root, agentDir := filepath.Join(base, "workspace"), filepath.Join(base, "agent")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			realRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			global := `{"compaction":{"reserveTokens":10000,"keepRecentTokens":5000,"modelOverrides":{"provider/nested/model":{"reserveTokens":20000,"keepRecentTokens":2000}}}`
			if tc.globalTrust != "" {
				global += `,"defaultProjectTrust":` + tc.globalTrust
			}
			writeAgentContextFixture(t, filepath.Join(agentDir, "settings.json"), global+`}`)
			project := tc.project
			if project == "" {
				project = `{"compaction":{"reserveTokens":8000,"modelOverrides":{"provider/nested/model":{"keepRecentTokens":1000}}}}`
			}
			writeAgentContextFixture(t, filepath.Join(root, ".pi", "settings.json"), project)
			if tc.trust != "" {
				trust := map[string]any{filepath.Dir(realRoot): true}
				switch tc.trust {
				case "nearest-deny":
					trust[realRoot] = false
				case "null-nearest":
					trust[realRoot] = nil
				}
				raw, _ := json.Marshal(trust)
				if tc.trust == "invalid" {
					raw = []byte(`{`)
				}
				writeAgentContextFixture(t, filepath.Join(agentDir, "trust.json"), string(raw))
			}
			config := settings.AgentServer{Command: "npx", Args: []string{"-y", "pi-acp@0.0.34"}, Env: map[string]string{"HOME": base, "PI_CODING_AGENT_DIR": agentDir}}
			policy := loadAgentCompactionPolicy(config, root)
			known, _, reserve := policy.forModel("other")
			if known != tc.known || reserve != tc.reserve {
				t.Fatalf("global policy = known:%v reserve:%d, want %v %d", known, reserve, tc.known, tc.reserve)
			}
			known, _, reserve = policy.forModel("provider/nested/model")
			if known != tc.known || reserve != tc.modelReserve {
				t.Fatalf("model policy = known:%v reserve:%d, want %v %d", known, reserve, tc.known, tc.modelReserve)
			}
		})
	}
}

func TestAgentContextJSONFileBounds(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, text string
		known      bool
	}{
		{name: "missing", known: true},
		{name: "valid", text: `{}`, known: true},
		{name: "malformed", text: `{`},
		{name: "at limit", text: `{}` + strings.Repeat(" ", (1<<20)-2), known: true},
		{name: "over limit", text: `{}` + strings.Repeat(" ", (1<<20)-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if tc.text != "" {
				writeAgentContextFixture(t, path, tc.text)
			}
			var value agentPiSettings
			if got := readAgentContextJSON(path, &value); got != tc.known {
				t.Fatalf("read returned %v, want %v", got, tc.known)
			}
		})
	}
	for _, path := range append([]string{dir}, specialFiles(t, dir)...) {
		t.Run("special "+filepath.Base(path), func(t *testing.T) {
			done := make(chan bool, 1)
			go func() {
				var value agentPiSettings
				done <- readAgentContextJSON(path, &value)
			}()
			select {
			case got := <-done:
				if got {
					t.Fatal("nonregular file was accepted as settings")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("settings read blocked on a nonregular file")
			}
		})
	}
}

func TestAgentContextPiSpecialProjectFiles(t *testing.T) {
	files := specialFiles(t, t.TempDir())
	if len(files) == 0 {
		t.Skip("platform has no special-file fixtures")
	}
	for _, path := range files {
		for _, trust := range []string{"never", "always"} {
			t.Run(filepath.Base(path)+" "+trust, func(t *testing.T) {
				root, agentDir := t.TempDir(), t.TempDir()
				writeAgentContextFixture(t, filepath.Join(agentDir, "settings.json"), `{"compaction":{"reserveTokens":1234},"defaultProjectTrust":"`+trust+`"}`)
				if err := os.Mkdir(filepath.Join(root, ".pi"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(root, ".pi", "settings.json")); err != nil {
					t.Fatal(err)
				}
				config := settings.AgentServer{Command: "pi-acp", Env: map[string]string{"PI_CODING_AGENT_DIR": agentDir}}
				done := make(chan agentCompactionPolicy, 1)
				go func() { done <- loadAgentCompactionPolicy(config, root) }()
				select {
				case policy := <-done:
					if trust == "always" && policy.known {
						t.Fatal("trusted nonregular project settings did not produce an unknown policy")
					}
					if trust == "never" && (!policy.known || policy.reserveTokens != 1234) {
						t.Fatal("untrusted nonregular project settings affected the global policy")
					}
				case <-time.After(2 * time.Second):
					t.Fatal("policy loading blocked on project settings")
				}
			})
		}
	}
}

func TestAgentContextPiPathsAndUnknownProviders(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	for _, tc := range []struct{ name, directory, path string }{
		{"default", "", filepath.Join(home, ".pi", "agent")},
		{"relative", "relative-agent", filepath.Join(root, "relative-agent")},
		{"home expansion", "~/custom-agent", filepath.Join(home, "custom-agent")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeAgentContextFixture(t, filepath.Join(tc.path, "settings.json"), `{"compaction":{"reserveTokens":1234}}`)
			config := settings.AgentServer{Command: "/bin/pi-acp", WorkingDirectory: "ignored-adapter-directory", Env: map[string]string{"HOME": home, "PI_CODING_AGENT_DIR": tc.directory}}
			policy := loadAgentCompactionPolicy(config, root)
			if !policy.known || policy.reserveTokens != 1234 {
				t.Fatalf("Pi process settings path was not resolved: %+v", policy)
			}
			config.Command = "another-agent"
			if loadAgentCompactionPolicy(config, root).known {
				t.Fatal("another adapter inherited Pi's compaction policy")
			}
		})
	}
}

func contextCategoryForTest(t *testing.T, snapshot agentContextSnapshot, kind string) agentContextCategory {
	t.Helper()
	for _, category := range snapshot.categories {
		if category.kind == kind {
			return category
		}
	}
	t.Fatalf("missing category %q", kind)
	return agentContextCategory{}
}

func TestAgentContextEstimatesOnlyActiveBoundedPrompt(t *testing.T) {
	s := &agentChat{sessionID: "session", generation: 3, contextMessages: 2, systemPrompt: "custom instructions",
		messages:     []*agentMessage{{role: "user", text: strings.Repeat("old history", 1000)}, {role: "assistant", text: "kept answer"}, {role: "user", text: "current question"}},
		contextUsage: &acp.UsageState{Used: 1000, Size: 100000}}
	prompt := captureAgentContext(s, chatContext(s))
	if strings.Contains(prompt, "old history") || !strings.Contains(prompt, "current question") {
		t.Fatal("the accounting hook changed the actual bounded prompt")
	}
	tool := &agentTool{sessionID: s.sessionID, generation: s.generation, title: "Read", rawInput: json.RawMessage(`{"path":"fixture"}`), text: "result"}
	s.tools = []*agentTool{tool, {sessionID: "old", generation: 3, text: strings.Repeat("old tool", 1000)}}
	s.messages = append(s.messages, &agentMessage{role: "thought", text: "visible thought"}, &agentMessage{role: "tool", tool: tool}, &agentMessage{role: "assistant", text: "new answer"})
	snapshot := (&app{}).contextSnapshot(s)
	if !snapshot.known || !snapshot.estimated || snapshot.used != 1000 {
		t.Fatalf("actual usage or estimate provenance lost: %+v", snapshot)
	}
	for _, kind := range []string{"system", "instructions", "skills", "builtin_tools", "custom_tools"} {
		if contextCategoryForTest(t, snapshot, kind).known {
			t.Fatalf("private adapter category %q was invented", kind)
		}
	}
	wantUser := agentContextTextTokens(roundtable.FormatPrompt([]roundtable.Message{{Role: "system", Text: "custom instructions"}})) +
		agentContextTextTokens(roundtable.FormatPrompt([]roundtable.Message{{Role: "user", Text: "current question"}}))
	if got := contextCategoryForTest(t, snapshot, "user"); !got.known || got.tokens != wantUser {
		t.Fatalf("user estimate included old transcript or lost custom instructions: %+v, want %d", got, wantUser)
	}
	if got := contextCategoryForTest(t, snapshot, "tool_output"); !got.known || got.tokens != agentContextTextTokens("result") {
		t.Fatalf("tool output counted shared pointers or stale sessions: %+v", got)
	}
	for _, used := range []int{0, 7, 1000} {
		s.contextUsage.Used = used
		snapshot = (&app{}).contextSnapshot(s)
		total := 0
		for _, category := range snapshot.categories {
			if category.tokens < 0 || category.tokens > used {
				t.Fatalf("estimate exceeds real occupancy %d: %+v", used, category)
			}
			total += category.tokens
		}
		if total != used {
			t.Fatalf("estimates and unattributed remainder sum to %d, want real usage %d", total, used)
		}
	}
	for _, stale := range []string{"session", "generation"} {
		copy := *s
		if stale == "session" {
			copy.sessionID = "new"
		} else {
			copy.generation++
		}
		got := (&app{}).contextSnapshot(&copy)
		if got.estimated || contextCategoryForTest(t, got, "unattributed").tokens != copy.contextUsage.Used {
			t.Fatalf("stale %s accounting reached the new context", stale)
		}
	}
}

func configureAgentContextPolicyFixture(t *testing.T, hs *harness) string {
	t.Helper()
	agentDir := t.TempDir()
	settingsPath := filepath.Join(agentDir, "settings.json")
	writeAgentContextFixture(t, settingsPath, `{"compaction":{"reserveTokens":12000}}`)
	provider := hs.a.settings.AgentServers["backend"]
	provider.Args = append(provider.Args, "pi-acp@fixture")
	provider.Env = maps.Clone(provider.Env)
	provider.Env["PI_CODING_AGENT_DIR"] = agentDir
	hs.a.settings.AgentServers["backend"] = provider
	return settingsPath
}

func TestAgentContextPolicyReconnectAndReset(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	a, s := hs.a, hs.a.agent
	settingsPath := configureAgentContextPolicyFixture(t, hs)
	connectAgentModelTest(t, hs)
	if !s.contextPolicy.known || s.contextPolicy.reserveTokens != 12000 {
		t.Fatalf("connection did not capture the Pi policy: %+v", s.contextPolicy)
	}
	writeAgentContextFixture(t, settingsPath, `{"compaction":{"reserveTokens":4000}}`)
	if s.contextPolicy.reserveTokens != 12000 {
		t.Fatal("a local file change mutated the active connection snapshot")
	}
	a.setAgentModel("model-b")
	hs.untilLong("model selection keeps session policy", func() bool { return !s.configuring })
	if s.contextPolicy.reserveTokens != 12000 {
		t.Fatal("model selection reloaded a running Pi session's settings")
	}
	s.contextMessages = 1
	lifecycleSend(t, hs, "first session question")
	firstSession := s.sessionID
	if s.contextAccounting.session != firstSession || s.contextPolicy.reserveTokens != 12000 {
		t.Fatal("first prompt lost its session policy or accounting")
	}
	lifecycleSend(t, hs, "fresh session question")
	if s.sessionID == firstSession || s.contextAccounting.session != s.sessionID || s.contextPolicy.reserveTokens != 4000 {
		t.Fatal("fresh ACP session did not refresh its policy and accounting")
	}
	wantUser := agentContextTextTokens(roundtable.FormatPrompt([]roundtable.Message{{Role: "system", Text: s.connectedRole.Prompt}})) +
		agentContextTextTokens(roundtable.FormatPrompt([]roundtable.Message{{Role: "user", Text: "fresh session question"}}))
	if s.contextAccounting.input["user"] != wantUser || s.contextAccounting.input["assistant"] != 0 {
		t.Fatal("fresh session accounting retained history outside its one-message limit")
	}
	s.draft, s.contextDetailsOpen = "keep draft", true
	s.contextAccounting = agentContextAccounting{session: s.sessionID, generation: s.generation, input: map[string]int{"user": 20}}
	a.disconnectChat(s)
	if s.contextDetailsOpen || s.contextPolicy.known || s.contextAccounting.session != "" || s.draft != "keep draft" {
		t.Fatal("disconnect retained context metadata or changed the draft")
	}
	connectAgentModelTest(t, hs)
	if !s.contextPolicy.known || s.contextPolicy.reserveTokens != 4000 {
		t.Fatal("reconnect did not capture the updated Pi policy")
	}
	s.contextDetailsOpen = true
	a.newAgentSession()
	if s.contextDetailsOpen || s.contextPolicy.known || s.contextAccounting.session != "" || s.draft != "keep draft" {
		t.Fatal("new chat retained context metadata or changed the draft")
	}
}

func TestAgentContextPolicySurvivesSessionModelFailure(t *testing.T) {
	for _, scenario := range []string{"second-model-removed", "auth-model-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			hs, logPath := agentRoleHarness(t, scenario)
			a, s := hs.a, hs.a.agent
			settingsPath := configureAgentContextPolicyFixture(t, hs)
			previousSession, wantPrompts := "", 0
			if scenario == "second-model-removed" {
				connectAgentModelTest(t, hs)
				a.setAgentModel("model-b")
				hs.untilLong("first session model", func() bool { return !s.configuring })
				lifecycleSend(t, hs, "first session succeeds")
				if s.err != "" {
					t.Fatal(s.err)
				}
				previousSession, wantPrompts = s.sessionID, 1
				s.contextUsage = &acp.UsageState{Used: 500, Size: 100000}
			} else {
				a.showAgent()
				a.connectAgent()
				hs.untilLong("authentication required", func() bool { return !s.connecting })
				if s.err == "" || s.client == nil || s.sessionID != "" {
					t.Fatal("fixture did not require authentication before session creation")
				}
			}
			writeAgentContextFixture(t, settingsPath, `{"compaction":{"reserveTokens":4000}}`)
			s.draft = "keep this question"
			if scenario == "second-model-removed" {
				lifecycleSend(t, hs, s.draft)
			} else {
				a.authenticateAgent("login")
				hs.untilLong("authenticated model rejection", func() bool { return !s.connecting })
			}
			if s.err == "" || s.sessionID == "" || s.sessionID == previousSession || s.draft != "keep this question" {
				t.Fatalf("model rejection did not retain the new recoverable session and draft: session=%q error=%q draft=%q", s.sessionID, s.err, s.draft)
			}
			if !s.contextPolicy.known || s.contextPolicy.reserveTokens != 4000 || s.contextAccounting.session != "" || s.contextUsage != nil {
				t.Fatal("model rejection lost the new session policy or retained old context accounting")
			}
			s.contextUsage = &acp.UsageState{Used: 100, Size: 100000}
			if snapshot := a.contextSnapshot(s); !snapshot.compactionKnown || snapshot.compactAt != 96000 {
				t.Fatalf("recoverable session has no matching compaction threshold: %+v", snapshot)
			}
			prompts := 0
			for _, event := range agentRoleEvents(t, logPath) {
				if event.Kind == "prompt" {
					prompts++
				}
			}
			if prompts != wantPrompts {
				t.Fatal("model rejection still sent the pending question")
			}
		})
	}
}

func TestAgentContextSnapshotUsesConfirmedModel(t *testing.T) {
	s := &agentChat{currentModel: "provider/current", modelOverride: "provider/unavailable", contextUsage: &acp.UsageState{Used: 100, Size: 10000},
		models:        []acp.ModelChoice{{ID: "provider/current", Name: "Current Model"}},
		contextPolicy: parseAgentCompactionPolicy(json.RawMessage(`{"reserveTokens":3000,"modelOverrides":{"provider/current":{"reserveTokens":1000}}}`))}
	got := (&app{}).contextSnapshot(s)
	if got.model != "Current Model" || got.compactAt != 9000 {
		t.Fatalf("a stale requested model replaced the server-confirmed model: %+v", got)
	}
	s.contextUsage = nil
	if got := (&app{}).contextSnapshot(s); got.known || got.estimated {
		t.Fatal("unreported usage was reconstructed from local transcript")
	}
}
