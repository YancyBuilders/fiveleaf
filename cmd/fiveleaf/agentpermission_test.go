package main

import (
	"context"
	"encoding/json"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/acp"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
)

func TestAgentConfirmationCard(t *testing.T) {
	const question = "允许 Pi 发送本次演示回复吗？"
	for _, view := range []struct {
		name, locale string
		width        int
		dark         bool
		answer       string
	}{
		{"desktop", "zh-Hans", 1400, false, "yes"},
		{"compact", "zh-Hans", 350, true, "no"},
		{"split", "ja", 180, false, ""},
	} {
		t.Run(view.name, func(t *testing.T) {
			before := i18n.Locale()
			i18n.SetLocaleFunc(func() string { return view.locale })
			t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return before }) })
			hs := newHarness(t, view.dark)
			agentHome(t, hs.root)
			hs.a.showSessions()
			hs.a.settings.AgentServers = map[string]settings.AgentServer{"pi": {Name: "Pi", Protocol: "cli", Command: "unused"}}
			hs.a.openAgentPage()
			chat := hs.a.agent
			chat.busy, chat.sessionID = true, "approval"
			chat.messages = []*agentMessage{{role: "user", text: "后台通知体验"}}
			hs.a.window.SidebarClosed = view.width < 500
			hs.tt.SetLocale(view.locale)
			hs.tt.SetSize(view.width, 760)
			if view.name == "desktop" {
				hs.tt.SetScale(2)
			}
			var request acp.PermissionRequest
			err := json.Unmarshal([]byte(`{"sessionId":"approval","toolCall":{"toolCallId":"notice","title":"通知体验","rawInput":{"method":"confirm","title":"通知体验","message":"`+question+`"}},"options":[{"optionId":"yes","kind":"allow_once","name":"Yes"},{"optionId":"no","kind":"reject_once","name":"No"}]}`), &request)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			handler := hs.a.chatHandler(chat)
			outcomes := make(chan acp.PermissionOutcome, 1)
			go func() {
				outcome, _ := handler.RequestPermission(ctx, request)
				outcomes <- outcome
			}()
			hs.until("confirmation card", func() bool { return len(chat.permissions) == 1 })
			hs.settle()
			if !hs.tt.HasText(question) || hs.tt.HasText(`"method"`) || hs.tt.HasText(locales.AgentWorking()) {
				t.Fatalf("expected the question and waiting state: %v", hs.tt.Texts())
			}
			labels := []string{locales.AgentPermissionYes(), locales.AgentPermissionNo(), locales.ButtonCancel(), locales.AgentPermissionDetails()}
			card := hs.detailBounds(locales.AgentPermission())
			for _, label := range labels {
				bounds := hs.detailBounds(label)
				if bounds.X < card.X || bounds.X+bounds.W > card.X+card.W+1 || bounds.Y < card.Y || bounds.Y+bounds.H > card.Y+card.H+1 || bounds.W > 165 {
					t.Fatalf("%s left its card or stretched: %v in %v", label, bounds, card)
				}
			}
			if view.width > 300 {
				yes, no := hs.detailBounds(labels[0]), hs.detailBounds(labels[1])
				if yes.Y != no.Y || yes.X+yes.W >= no.X || card.W > 480 {
					t.Fatalf("approval actions should sit side by side: %v %v in %v", yes, no, card)
				}
			}
			hs.shot("approval-" + view.name)
			if view.name == "desktop" {
				hs.shotRect("approval-card", image.Rect(int(card.X)-12, int(card.Y)-12, int(card.X+card.W)+12, int(card.Y+card.H)+12), 2)
			}
			if err := hs.tt.Click(locales.AgentPermissionDetails()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if !hs.tt.HasText(`"method": "confirm"`) || hs.tt.Click(locales.AgentPermissionCopy()) != nil || !strings.Contains(hs.tt.Clipboard(), question) {
				t.Fatal("the original request must be inspectable and copyable")
			}
			if err := hs.tt.Click(locales.AgentPermissionDetails()); err != nil {
				t.Fatal(err)
			}
			label := locales.ButtonCancel()
			if view.answer == "yes" {
				label = locales.AgentPermissionYes()
			} else if view.answer == "no" {
				label = locales.AgentPermissionNo()
			}
			if err := hs.tt.Click(label); err != nil {
				t.Fatal(err)
			}
			hs.until("permission removal", func() bool { return len(chat.permissions) == 0 })
			select {
			case outcome := <-outcomes:
				if view.answer == "" && outcome.Cancelled == nil || view.answer != "" && (outcome.Selected == nil || string(outcome.Selected.OptionId) != view.answer) {
					t.Fatalf("decision %q returned %+v", view.answer, outcome)
				}
			case <-time.After(time.Second):
				t.Fatal("the approval did not return")
			}
		})
	}
}

func TestAgentPermissionScopeAndReadingPosition(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	chat := hs.a.agent
	chat.busy = true
	chat.messages = []*agentMessage{{role: "assistant", text: strings.Repeat("Earlier paragraph.\n\n", 45)}}
	hs.settle()
	previousEnd := chat.scroll.MaxY
	var answers []string
	request := &agentPermission{title: "Allow edits?", options: []acp.PermissionOption{
		{OptionId: "project", Kind: "allow_always", Name: "Trust this project"},
		{OptionId: "once", Kind: "allow_once", Name: "Allow this edit"},
		{OptionId: "never", Kind: "reject_always", Name: "Block edits in this project"},
	}, choose: func(id string) { answers = append(answers, id) }}
	chat.permissions = append(chat.permissions, request)
	hs.settle()
	if chat.scroll.MaxY <= previousEnd || chat.scroll.Y < chat.scroll.MaxY-1 {
		t.Fatal("new permission did not follow the end of the conversation")
	}
	once := hs.detailBounds("Allow this edit")
	future := hs.detailBounds(locales.AgentPermissionFuture())
	always := hs.detailBounds("Trust this project")
	if future.Y < once.Y+once.H || always.Y < future.Y+future.H || len(answers) != 0 {
		t.Fatal("persistent options must be separate and require an explicit choice")
	}
	for range 2 {
		if err := hs.tt.Click("Trust this project"); err != nil {
			t.Fatal(err)
		}
		hs.settle()
	}
	if !slices.Equal(answers, []string{"project"}) {
		t.Fatalf("expected only the selected option ID once, got %v", answers)
	}
	chat.scroll.Y = 0
	hs.settle()
	chat.permissions = append(chat.permissions, &agentPermission{title: "Another request"})
	hs.settle()
	if chat.scroll.Y != 0 {
		t.Fatal("a later permission moved the reader away from earlier messages")
	}
}

func TestAgentPermissionPreviewCoverage(t *testing.T) {
	for _, test := range []struct {
		input    string
		complete bool
	}{
		{`{"method":"confirm","title":"Send","message":"Send?"}`, true},
		{`{"method":"confirm","message":"Send?","destination":"external"}`, false},
		{`{"method":"confirm","message":"Send?","title":{"extra":true}}`, false},
		{`{"command":"go test ./..."}`, true},
		{`{"command":"go","args":["test"],"cwd":"/workspace"}`, false},
		{`{"path":"src/main.go"}`, true},
		{`{"filePath":"src/main.go","content":"changes"}`, false},
	} {
		preview := previewPermission(json.RawMessage(test.input), nil, "Send", false)
		if preview.complete != test.complete || preview.complete && preview.question == "" && preview.target == "" {
			t.Errorf("wrong initial detail visibility for %s: %+v", test.input, preview)
		}
		if previewPermission(json.RawMessage(test.input), nil, "Send", true).complete || previewPermission(json.RawMessage(test.input), []agentLocation{{Path: "file"}}, "Send", false).complete {
			t.Errorf("preview hid additional tool content or locations for %s", test.input)
		}
	}
}
