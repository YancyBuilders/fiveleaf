//go:build agentintegration

package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type liveAgentConfig struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Server         settings.AgentServer `json:"server"`
	Model          string               `json:"model"`
	AlternateModel string               `json:"alternateModel"`
	ReplyMarker    string               `json:"replyMarker"`
	ThoughtHigh    string               `json:"thoughtHigh"`
	RestoreDefault bool                 `json:"restoreDefault"`
}

// Run explicitly with -tags agentintegration and FIVELEAF_AGENT_LIVE_CONFIG
// pointing to {"agents":[...]}. Each server must use isolated provider state.
// The UI receives an empty workspace and never approves tool permissions.
func TestAgentLiveWorkflow(t *testing.T) {
	path := os.Getenv("FIVELEAF_AGENT_LIVE_CONFIG")
	if path == "" {
		t.Skip("set FIVELEAF_AGENT_LIVE_CONFIG to an isolated live agent configuration")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read live agent configuration")
	}
	var config struct {
		Agents []liveAgentConfig `json:"agents"`
	}
	if err := json.Unmarshal(raw, &config); err != nil || len(config.Agents) == 0 {
		t.Fatal("live agent configuration must contain agents")
	}
	for _, agent := range config.Agents {
		t.Run(agent.ID, func(t *testing.T) { testAgentLiveWorkflow(t, agent) })
	}
}

func testAgentLiveWorkflow(t *testing.T, config liveAgentConfig) {
	if config.ID == "" || config.Name == "" || config.Model == "" || config.AlternateModel == "" ||
		config.AlternateModel == config.Model || config.ThoughtHigh == "" || config.ReplyMarker == "" {
		t.Fatal("live agent configuration requires a name, two models, thinking level and reply marker")
	}
	root := t.TempDir()
	s := settings.Default()
	s.MarkdownMode = ""
	config.Server.WorkingDirectory = root
	config.Server.TimeoutSeconds = 90
	s.AgentServers[config.ID] = config.Server
	role := config.Name + " live test"
	s.AgentRoles[role] = settings.AgentRole{Agent: config.ID, Model: config.Model}
	w := s.NewWindow()
	w.Folder = root
	h := &fakeHost{folder: root}
	a := newApp(h, s, w)
	t.Cleanup(a.shutdown)
	tt := ui.NewTester(a.view, 1200, 760)
	tt.SetDark(true)
	hs := &harness{t: t, a: a, h: h, tt: tt, root: root}
	hs.settle()
	a.selectAgentRole(role)
	a.openAgentPage()
	hs.settle()
	chat := a.agent

	checkSafe := func(phase string) {
		t.Helper()
		if len(chat.permissions) != 0 || len(chat.tools) != 0 {
			t.Fatalf("%s: agent requested a tool or permission during the no-tools probe", phase)
		}
		if chat.err != "" {
			// Provider errors may echo private configuration. Only public state is
			// logged; neither raw errors nor process environments enter test logs.
			t.Fatalf("%s: agent reported an error (content withheld); busy=%v connecting=%v models=%d messages=%d",
				phase, chat.busy, chat.connecting, len(chat.models), len(chat.messages))
		}
	}
	wait := func(phase string, done func() bool) {
		t.Helper()
		started := time.Now()
		deadline := started.Add(90 * time.Second)
		for !done() {
			step(tt)
			checkSafe(phase)
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s; busy=%v connecting=%v models=%d messages=%d",
					phase, chat.busy, chat.connecting, len(chat.models), len(chat.messages))
			}
			time.Sleep(20 * time.Millisecond)
		}
		checkSafe(phase)
		hs.settle()
		checkSafe(phase)
		t.Logf("%s completed in %s", phase, time.Since(started).Round(time.Millisecond))
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("live workflow could not click %q", label)
		}
		step(tt)
	}
	wait("connection", func() bool { return chat.client != nil && chat.sessionID != "" && !chat.connecting })
	client := chat.client
	assertModel := func(id string) {
		t.Helper()
		confirmed, ok := client.ModelState(chat.sessionID)
		if !ok || confirmed.Current != id || chat.currentModel != id || chat.model() != id {
			t.Fatal("visible model does not match the active ACP session")
		}
	}
	assertThinking := func(id string) {
		t.Helper()
		confirmed, ok := client.ModeState(chat.sessionID)
		if !ok || !confirmed.Thinking || confirmed.Current != id || chat.mode != id {
			t.Fatal("visible thinking choice does not match the active ACP session")
		}
	}
	assertModel(config.Model)
	if initial, ok := client.UsageState(chat.sessionID); ok {
		if chat.contextUsage == nil || *chat.contextUsage != initial {
			t.Fatal("initial context differs from the active ACP session")
		}
	} else if chat.contextUsage != nil || a.contextSnapshot(chat).known {
		t.Fatal("the new session fabricated context before an agent report")
	}
	hs.shot(config.ID + "-initial")

	chooseThinking := func(id string) {
		t.Helper()
		choices := agentThinkingModes(chat)
		index := slices.IndexFunc(choices, func(choice agentChoice) bool { return choice.id == id })
		if index < 0 || len(choices) < 2 {
			t.Fatal("the agent did not advertise the requested thinking level")
		}
		click(locales.AgentModel())
		hs.settle()
		rail, found := tt.Find(agentThinkingName)
		if !found {
			t.Fatal("thinking control is missing inside the model picker")
		}
		tt.ClickAt(rail.X+12+(rail.W-24)*float32(index)/float32(len(choices)-1), rail.Y+rail.H/2)
		step(tt)
		wait("thinking selection", func() bool {
			return !chat.configuring && chat.modelPicker.thinking.pending == nil && chat.mode == id
		})
		assertThinking(id)
		suffix := "-thinking-high"
		if id == "" {
			suffix = "-thinking-default"
		}
		hs.shot(config.ID + suffix)
		if chat.modelPicker.open {
			hs.key(0, ui.KeyEscape)
			hs.settle()
		}
	}
	chooseThinking(config.ThoughtHigh)
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
			t.Fatal("the agent did not advertise the configured model")
		}
		click(locales.AgentModel())
		tt.Type(id)
		step(tt)
		click(label)
		wait("model selection", func() bool { return !chat.configuring && chat.modelOverride == id && chat.currentModel == id })
		assertModel(id)
		if chat.modelPicker.open {
			hs.key(0, ui.KeyEscape)
			hs.settle()
		}
		if s.AgentRoles[role].Model != config.Model {
			t.Fatal("changing the conversation model changed its saved role default")
		}
	}
	chooseModel(config.AlternateModel)
	chooseModel(config.Model)
	// Some agents restore a model's own thinking default when switching. The
	// displayed choice must follow that response, including an empty value.
	assertThinking(chat.mode)
	t.Logf("model round trip confirmed thinking=%q", chat.mode)
	hs.shot(config.ID + "-model-roundtrip")
	expectedThinking := config.ThoughtHigh
	if config.RestoreDefault {
		chooseThinking(config.ThoughtHigh)
		chooseThinking("")
		expectedThinking = ""
	} else {
		assertThinking(config.ThoughtHigh)
	}
	assertContext := func(suffix string) {
		t.Helper()
		wait("context report", func() bool {
			return chat.contextUsage != nil && chat.contextUsage.Used > 0 && chat.contextUsage.Size > 0
		})
		usage := *chat.contextUsage
		if confirmed, ok := client.UsageState(chat.sessionID); !ok || confirmed != usage {
			t.Fatal("visible context does not match the active ACP session")
		}
		percent := agentContextPercentage(usage)
		if !tt.HasText(percent+"%") || !tt.HasText(agentTokenCount(usage.Size)) {
			t.Fatal("context bar omitted the reported capacity or percentage")
		}
		hs.shot(config.ID + "-usage" + suffix)
		click(locales.AgentContext())
		hs.settle()
		if !tt.HasText(agentContextTokenCount(usage.Used)) ||
			!tt.HasText(locales.AgentContextUsageSummary(agentContextTokenCount(usage.Size), percent)) {
			t.Fatal("context details differ from the usage shown above the input")
		}
		details := a.contextSnapshot(chat)
		if !details.known || details.used != usage.Used || details.size != usage.Size {
			t.Fatal("context details do not describe the current session report")
		}
		if !isPiACP(config.Server) && details.compactionKnown {
			t.Fatal("a non-Pi agent inherited an unsupported compaction threshold")
		}
		hs.shot(config.ID + "-context-details" + suffix)
		click(locales.ButtonClose())
		hs.settle()
		t.Logf("reported context: used=%d size=%d (%s%%)", usage.Used, usage.Size, percent)
	}
	send := func(marker, suffix string) {
		t.Helper()
		before := len(chat.messages)
		lastReply := func() string {
			for i := len(chat.messages) - 1; i >= before; i-- {
				if chat.messages[i].role == "assistant" {
					return chat.messages[i].text
				}
			}
			return ""
		}
		chat.draft = "Do not use any tools. Reply with exactly " + marker + " and no other text."
		hs.settle()
		click(locales.AgentSend())
		wait("controlled no-tools reply", func() bool { return !chat.busy && lastReply() != "" })
		if strings.TrimSpace(lastReply()) != marker {
			t.Fatal("the agent did not return the exact controlled marker (reply withheld)")
		}
		if a.agent != chat || chat.client != client || !a.isAgentPageVisible() {
			t.Fatal("the conversation lost its page or ACP connection")
		}
		assertModel(config.Model)
		assertThinking(expectedThinking)
		assertContext(suffix)
	}
	send(config.ReplyMarker, "")
	if config.RestoreDefault {
		firstSession := chat.sessionID
		send(config.ReplyMarker+"_FOLLOWUP", "-followup")
		if chat.sessionID == firstSession {
			t.Fatal("the provider-default replay probe did not create a fresh ACP session")
		}
	}
	t.Logf("%s: thinking control, model round trip, exact reply and session context passed; confirmed model=%q thinking=%q",
		config.Name, config.Model, expectedThinking)
}
