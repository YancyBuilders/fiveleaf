//go:build piintegration

package main

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

// Run explicitly with -tags piintegration and FIVELEAF_PI_SMOKE_CONFIG pointing
// to {"env":{...},"model":"provider/model"}. The isolated Pi configuration must
// gate tool calls through ui.confirm; this test approves only its fixture edit.
func TestPiAgentLiveWorkflow(t *testing.T) {
	var config struct {
		Env            map[string]string `json:"env"`
		Model          string            `json:"model"`
		AlternateModel string            `json:"alternateModel"`
	}
	raw, err := os.ReadFile(os.Getenv("FIVELEAF_PI_SMOKE_CONFIG"))
	if err != nil {
		t.Fatal("set FIVELEAF_PI_SMOKE_CONFIG to an isolated Pi smoke configuration")
	}
	if err := json.Unmarshal(raw, &config); err != nil || config.Model == "" {
		t.Fatal("Pi smoke configuration requires a model")
	}
	ph := prefsAgentHarness(t)
	prefsAgentClick(t, ph, locales.PrefsAgentAdd())
	d := ph.w.agentDraft
	d.name, d.command, d.args = "Pi", "npx", []string{"-y", "pi-acp@0.0.34"}
	if path, err := exec.LookPath("npx"); err == nil {
		d.command = path
	}
	for name, value := range config.Env {
		d.env = append(d.env, prefAgentVariable{name, value})
	}
	d.timeout = "180"
	ph.frames()
	if bounds, ok := ph.pt.Find(locales.ButtonSave()); ok && bounds.Y > 850 {
		ph.pt.Scroll(700, 600, 0, bounds.Y-600)
		ph.frames()
	}
	prefsAgentClick(t, ph, locales.ButtonSave())
	if ph.w.agentDraft != nil {
		t.Fatal("Pi registration did not save")
	}
	preset := &agentRolePresets[2]
	ph.w.editPresetRole(preset)
	ph.w.roleDraft.agent, ph.w.roleDraft.model = "custom-pi", config.Model
	if !ph.w.saveAgentRole() {
		t.Fatal("Pi model role did not save")
	}
	ph.m.s.Flush()
	ph.w.query = ""
	ph.frames()
	ph.shot("pi-agent-settings")

	a := ph.a
	a.selectAgentTarget(a.agent, agentTarget{provider: "custom-pi", preset: preset})
	a.openAgentPage()
	ph.settle()
	chat := a.agent
	lastReply := func() string {
		for i := len(chat.messages) - 1; i >= 0; i-- {
			if chat.messages[i].role == "assistant" {
				return chat.messages[i].text
			}
		}
		return ""
	}
	diagnose := func() {
		t.Helper()
		publicSummary := func(text string, limit int) string {
			text = strings.Join(strings.Fields(text), " ")
			chars := []rune(text)
			if len(chars) > limit {
				return string(chars[:limit]) + "…"
			}
			return text
		}
		t.Logf("Pi state: busy=%v connecting=%v stopping=%v session=%q model=%q advertisedModels=%d permissions=%d messages=%d tools=%d turn=%d activeChat=%v pageVisible=%v",
			chat.busy, chat.connecting, chat.stopping, chat.sessionID, chat.model(), len(chat.models), len(chat.permissions), len(chat.messages), len(chat.tools), chat.turn, a.agent == chat, a.isAgentPageVisible())
		// Only the controlled fixture's public reply and tool labels are logged;
		// raw arguments, provider configuration and process environment stay out.
		t.Logf("Pi last public reply: %q", publicSummary(lastReply(), 600))
		for i := max(0, len(chat.tools)-4); i < len(chat.tools); i++ {
			tool := chat.tools[i]
			t.Logf("Pi tool[%d]: kind=%q status=%q title=%q", i, tool.kind, tool.status, publicSummary(tool.title, 160))
		}
	}
	wait := func(label string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(180 * time.Second)
		for !done() {
			step(ph.tt)
			if chat.err != "" {
				diagnose()
				t.Fatalf("%s: %s", label, chat.err)
			}
			if time.Now().After(deadline) {
				diagnose()
				t.Fatalf("timed out waiting for %s", label)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if chat.err != "" {
			diagnose()
			t.Fatalf("%s: %s", label, chat.err)
		}
		ph.settle()
	}
	wait("Pi connection", func() bool { return chat.client != nil && chat.sessionID != "" && !chat.connecting })
	if chat.model() != config.Model {
		t.Fatal("Pi connected without the configured model")
	}
	t.Logf("Pi ACP connected; model=%s", config.Model)
	client := chat.client
	send := func(text string) {
		t.Helper()
		chat.draft = text
		ph.settle()
		if err := ph.tt.Click(locales.AgentSend()); err != nil {
			t.Fatal(err)
		}
		step(ph.tt)
	}
	chooseModel := func(id string) {
		t.Helper()
		label := ""
		for _, choice := range chat.models {
			if choice.ID == id {
				label = agentModelLabel(chat.models, choice)
				break
			}
		}
		if label == "" {
			t.Fatalf("Pi did not advertise model %q", id)
		}
		if err := ph.tt.Click(locales.AgentModel()); err != nil {
			t.Fatal(err)
		}
		ph.tt.Type(id)
		step(ph.tt)
		if err := ph.tt.Click(label); err != nil {
			t.Fatal(err)
		}
		step(ph.tt)
		wait("Pi model selection", func() bool { return !chat.configuring && chat.modelOverride == id && chat.currentModel == id })
		if ph.m.s.AgentRolePresets[preset.id].Model != config.Model {
			t.Fatal("choosing a conversation model changed the role default")
		}
		t.Logf("Pi confirmed model selected through the picker: %s", id)
	}
	chooseThinking := func(id string) {
		t.Helper()
		if err := ph.tt.Click(locales.AgentModel()); err != nil {
			t.Fatal(err)
		}
		ph.settle()
		levels := agentThinkingModes(chat)
		index := slices.IndexFunc(levels, func(choice agentChoice) bool { return choice.id == id })
		if index < 0 || len(levels) < 2 {
			t.Fatalf("Pi did not advertise thinking level %q", id)
		}
		rail, found := ph.tt.Find(locales.AgentThinking())
		if !found {
			t.Fatal("thinking control is missing inside the model picker")
		}
		ph.tt.ClickAt(rail.X+12+(rail.W-24)*float32(index)/float32(len(levels)-1), rail.Y+rail.H/2)
		step(ph.tt)
		wait("Pi thinking selection", func() bool { return !chat.configuring && chat.mode == id })
		confirmed, ok := client.ModeState(chat.sessionID)
		if !ok || !confirmed.Thinking || confirmed.Current != id || !chat.modelPicker.open {
			t.Fatal("Pi thinking choice was not confirmed or closed its selector")
		}
		ph.harness.shot("pi-thinking-selected")
		ph.harness.key(0, ui.KeyEscape)
		ph.settle()
		t.Logf("Pi confirmed thinking selected through the picker: %s", id)
	}
	assertContext := func() {
		t.Helper()
		wait("Pi context report", func() bool {
			return chat.contextUsage != nil && chat.contextUsage.Used > 0 && chat.contextUsage.Size > 0
		})
		usage := *chat.contextUsage
		if snapshot, ok := client.UsageState(chat.sessionID); !ok || snapshot != usage {
			t.Fatal("visible context does not match the active ACP session")
		}
		percent := agentContextPercentage(usage)
		if !ph.tt.HasText(agentTokenCount(usage.Size)) {
			t.Fatal("Pi's context capacity was not rendered above the input")
		}
		if err := ph.tt.Click(locales.AgentContext()); err != nil {
			t.Fatal(err)
		}
		ph.settle()
		if !ph.tt.HasText(locales.AgentContextUsageSummary(agentContextTokenCount(usage.Size), percent)) || !ph.tt.HasText(agentContextTokenCount(usage.Used)) {
			t.Fatal("Pi's context details did not show its reported usage")
		}
		details := a.contextSnapshot(chat)
		if !details.compactionKnown || !details.compactionEnabled || details.compactAt != usage.Size-16384 {
			t.Fatal("Pi's default compaction threshold did not follow the selected model's capacity")
		}
		ph.harness.shot("pi-context-details")
		if err := ph.tt.Click(locales.ButtonClose()); err != nil {
			t.Fatal(err)
		}
		ph.settle()
		t.Logf("Pi reported context: used=%d size=%d (%s%%)", usage.Used, usage.Size, percent)
	}
	if config.AlternateModel == "" {
		provider, _, _ := strings.Cut(config.Model, "/")
		for _, choice := range chat.models {
			if choice.ID != config.Model && strings.HasPrefix(choice.ID, provider+"/") {
				config.AlternateModel = choice.ID
				break
			}
		}
	}
	if config.AlternateModel == "" || config.AlternateModel == config.Model {
		t.Fatal("the smoke configuration requires two different advertised models")
	}
	chooseModel(config.AlternateModel)
	send("Do not use tools. Reply with exactly PI_MODEL_OK.")
	wait("Pi reply with the selected model", func() bool { return !chat.busy && strings.Contains(lastReply(), "PI_MODEL_OK") })
	assertContext()
	ph.harness.shot("pi-model-selected")
	chooseModel(config.Model)
	chooseThinking("high")
	send("Do not use tools. Remember the codeword PINE-6831. Reply with exactly PI_CHAT_OK.")
	wait("first Pi reply", func() bool { return !chat.busy && lastReply() != "" })
	if !strings.Contains(lastReply(), "PI_CHAT_OK") {
		t.Fatal("Pi did not return the expected first reply")
	}
	t.Log("Pi returned PI_CHAT_OK through the workspace page")
	assertContext()
	if confirmed, ok := client.ModeState(chat.sessionID); !ok || confirmed.Current != "high" || chat.mode != "high" {
		t.Fatal("Pi model setup replaced the selected thinking level")
	}
	if err := os.WriteFile(ph.path("pi-smoke.txt"), []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	send("Use the write tool once to replace pi-smoke.txt in the current workspace with the remembered codeword from my previous message followed by a newline. Do not read or modify other files and do not use bash. Wait for permission when asked. Then reply with exactly PI_FILE_OK.")
	wait("Pi write permission", func() bool { return len(chat.permissions) > 0 })
	if data, err := os.ReadFile(ph.path("pi-smoke.txt")); err != nil || string(data) != "before\n" {
		t.Fatal("Pi changed the fixture before permission was granted")
	}
	ph.harness.shot("pi-agent-permission")
	a.openFile(ph.path("pi-smoke.txt"), true)
	ph.settle()
	if a.activeEditor() == nil {
		t.Fatal("Pi permission fixture did not switch to the editor")
	}
	a.showSessions()
	ph.settle()
	ph.harness.revealChatAwaitingPermission()
	if !a.isAgentPageVisible() || a.agent != chat || chat.client != client || len(chat.permissions) == 0 {
		t.Fatal("the Sessions menu did not reopen the original Pi chat with its pending permission")
	}
	permission := chat.permissions[0]
	chat.scroll.Y = math.MaxFloat32
	ph.settle()
	allowed := false
	for _, option := range permission.options {
		if option.OptionId == "yes" {
			// The adapter's permission is a real Pi extension ui.confirm, not a
			// synthetic tool request or an unconditional permission handler.
			if err := ph.tt.Click(permission.optionLabel(option)); err != nil {
				t.Fatal(err)
			}
			allowed = true
			break
		}
	}
	if !allowed {
		t.Fatal("Pi did not offer its expected confirm option")
	}
	wait("Pi file edit and follow-up reply", func() bool { return !chat.busy && strings.Contains(lastReply(), "PI_FILE_OK") })
	if data, err := os.ReadFile(ph.path("pi-smoke.txt")); err != nil || string(data) != "PINE-6831\n" {
		t.Fatal("Pi did not carry the conversation context into its approved file edit")
	}
	if chat.client != client || a.agent != chat || !a.isAgentPageVisible() {
		t.Fatal("Pi completion lost the original conversation or page")
	}
	ph.harness.shot("pi-agent-completed")
	if !ph.tt.HasText("PI_FILE_OK") {
		t.Fatal("Pi's final reply was not rendered")
	}
	assertContext()
	if confirmed, ok := client.ModeState(chat.sessionID); !ok || confirmed.Current != "high" || chat.mode != "high" {
		t.Fatal("Pi follow-up lost the selected thinking level")
	}
	t.Log("Pi follow-up preserved PINE-6831, waited for permission, wrote the fixture, and returned PI_FILE_OK")
}
