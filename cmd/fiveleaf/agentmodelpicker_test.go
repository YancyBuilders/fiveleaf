package main

import (
	"fmt"
	"slices"
	"testing"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestAgentModelPickerSearchAndGroups(t *testing.T) {
	choices := []acp.ModelChoice{
		{ID: "vendor-a/code-1", Name: "Code One", Description: "Fast editor", Group: "Coding"},
		{ID: "vendor-b/chat-2", Name: "Chat Two", Description: "Long conversations"},
		{ID: "vendor-a/code-3", Name: "Code Three", Description: "Detailed reasoning", Group: "Coding"},
		{ID: "local-model", Name: "Local"},
	}
	if got := agentModelGroups(choices, "adapter"); !slices.Equal(got, []string{"Coding", "vendor-b", "adapter"}) {
		t.Fatalf("server groups, ID prefixes or adapter fallback lost: %v", got)
	}
	for _, tc := range []struct {
		query, group string
		want         []string
	}{
		{query: "CODE", want: []string{"vendor-a/code-1", "vendor-a/code-3"}},
		{query: "chat-2", want: []string{"vendor-b/chat-2"}},
		{query: "long conversations", want: []string{"vendor-b/chat-2"}},
		{query: "code detailed", want: []string{"vendor-a/code-3"}},
		{group: "Coding", want: []string{"vendor-a/code-1", "vendor-a/code-3"}},
		{query: "missing"},
		{group: "vendor-b", query: "code"},
		{group: agentModelRecentGroup, want: []string{"vendor-a/code-3", "vendor-a/code-1"}},
	} {
		t.Run(tc.group+":"+tc.query, func(t *testing.T) {
			var got []string
			for _, choice := range agentModelMatches(choices, "adapter", tc.group, tc.query, []string{"removed", "vendor-a/code-3", "vendor-a/code-1"}) {
				got = append(got, choice.ID)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("matches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAgentModelPickerSearchKeyboardAndCancel(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	hs.a.openAgentPage()
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	s.draft = "keep this draft"
	s.models = []acp.ModelChoice{
		{ID: "model-a", Name: "Alpha", Description: "Precise work", Group: "One"},
		{ID: "model-b", Name: "Beta", Description: "Quick edits", Group: "Two"},
	}
	hs.settle()
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	hs.tt.Type("quick")
	hs.settle()
	if _, found := hs.tt.Find("Beta"); !found {
		t.Fatal("description search omitted the matching model")
	}
	if _, found := hs.tt.Find("Precise work"); found {
		t.Fatal("description search retained a nonmatching model")
	}
	hs.key(0, ui.KeyEnter)
	hs.untilLong("keyboard model choice", func() bool { return !s.configuring })
	hs.settle()
	if s.modelOverride != "model-b" || !s.modelPicker.open || !slices.Equal(s.recentModels, []string{"model-b"}) || s.draft != "keep this draft" || !hs.tt.Focused(locales.AgentModelSearch()) {
		t.Fatalf("keyboard choice did not apply, stay open and preserve the draft: model=%q open=%v recent=%v draft=%q focus=%v",
			s.modelOverride, s.modelPicker.open, s.recentModels, s.draft, hs.tt.Focused(locales.AgentModelSearch()))
	}
	hs.key(0, ui.KeyUp)
	hs.settle()
	if s.modelPicker.selected != "model-b" || s.model() != "model-b" {
		t.Fatalf("keyboard movement changed the active model: selected=%q active=%q", s.modelPicker.selected, s.model())
	}
	hs.key(0, ui.KeyEscape)
	hs.settle()
	if s.modelPicker.open || s.modelOverride != "model-b" || s.draft != "keep this draft" || !hs.tt.Focused(locales.AgentModel()) {
		t.Fatal("Escape changed the model or draft, or did not return focus to the model control")
	}
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	hs.tt.Type("not-a-model")
	hs.settle()
	if !hs.tt.HasText(locales.AgentModelNoMatches()) {
		t.Fatal("empty search has no explanation")
	}
	hs.key(0, ui.KeyEnter)
	hs.settle()
	if !s.modelPicker.open || s.modelOverride != "model-b" || s.requestInFlight() {
		t.Fatal("Enter on an empty search applied a stale model")
	}
	hs.key(0, ui.KeyEscape)
	hs.settle()
	if s.modelPicker.open {
		t.Fatal("Escape did not close the picker")
	}
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if s.modelPicker.query != "" || !hs.tt.Focused(locales.AgentModelSearch()) {
		t.Fatal("reopening the model picker did not reset and focus its search")
	}
	draft, found := hs.tt.Find(locales.AgentDraft())
	if !found {
		t.Fatal("draft is missing")
	}
	hs.tt.ClickAt(draft.X+draft.W-8, draft.Y+draft.H/2)
	hs.settle()
	if s.modelPicker.open || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatalf("outside dismissal stole focus from the draft: open=%v model=%v search=%v thinking=%v draft=%+v", s.modelPicker.open,
			hs.tt.Focused(locales.AgentModel()), hs.tt.Focused(locales.AgentModelSearch()), hs.tt.Focused(agentThinkingName), draft)
	}
}

func TestAgentModelPickerGroupSelectionAndHover(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	s.models = []acp.ModelChoice{{ID: "model-a", Name: "Alpha", Description: "First model", Group: "One"}, {ID: "model-b", Name: "Beta", Group: "Two"}}
	hs.settle()
	if err := hs.tt.Click(locales.AgentModel()); err != nil {
		t.Fatal(err)
	}
	r, ok := hs.tt.Find("Beta")
	if !ok {
		t.Fatal("second model missing")
	}
	hs.tt.Move(r.X+r.W/2, r.Y+r.H/2)
	hs.settle()
	if s.modelPicker.selected != "model-a" {
		t.Fatal("hover replaced keyboard selection")
	}
	if err := hs.tt.Click("Two"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if _, found := hs.tt.Find("First model"); found {
		t.Fatal("provider scope did not filter the list")
	}
	if s.modelPicker.selected != "model-b" || !hs.tt.Focused(locales.AgentModelSearch()) {
		t.Fatal("provider scope did not select its first row and restore search focus")
	}
	if err := hs.tt.Click(locales.AgentModelAllProviders()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if _, found := hs.tt.Find("First model"); !found {
		t.Fatal("all groups did not restore the complete list")
	}
}

func TestAgentModelPickerFitsWindowAndScrolls(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	hs.a.window.SidebarClosed = true
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	for i := range 30 {
		s.models = append(s.models, acp.ModelChoice{ID: fmt.Sprintf("extra-%02d", i), Name: fmt.Sprintf("Model %02d", i), Group: "Extra"})
	}
	for _, size := range [][2]int{{1000, 760}, {500, 480}, {340, 360}} {
		hs.tt.SetSize(size[0], size[1])
		hs.settle()
		if err := hs.tt.Click(locales.AgentModel()); err != nil {
			t.Fatal(err)
		}
		hs.settle()
		search, found := hs.tt.Find(locales.AgentModelSearch())
		anchor, _ := hs.tt.Find(locales.AgentModel())
		if !found || search.X < 0 || search.Y < 0 || search.X+search.W > float32(size[0]) || search.W > 500 || search.Y+search.H >= anchor.Y {
			t.Fatalf("picker does not fit or open above the anchor in %v: search=%+v anchor=%+v", size, search, anchor)
		}
		for range 35 {
			hs.key(0, ui.KeyDown)
		}
		hs.settle()
		last, found := hs.tt.Find("Model 29")
		hs.shot(fmt.Sprintf("agent-model-picker-%d", size[0]))
		if !found || last.Y < search.Y+search.H || last.Y+last.H > anchor.Y || last.W <= 0 {
			t.Fatalf("keyboard selection did not scroll the last row into view in %v: row=%+v search=%+v anchor=%+v (found=%v, selected=%q, searchFocused=%v)", size, last, search, anchor, found, s.modelPicker.selected, hs.tt.Focused(locales.AgentModelSearch()))
		}
		hs.key(0, ui.KeyEscape)
		hs.settle()
	}
}

func TestAgentModelPickerRejectsStaleSelection(t *testing.T) {
	for _, change := range []string{"role", "root", "generation", "session"} {
		t.Run(change, func(t *testing.T) {
			hs, _ := agentRoleHarness(t, "acp")
			connectAgentModelTest(t, hs)
			a, s := hs.a, hs.a.agent
			if err := hs.tt.Click(locales.AgentModel()); err != nil {
				t.Fatal(err)
			}
			a.chooseAgentModel(s, "model-b")
			switch change {
			case "role":
				a.selectAgent("backend")
			case "root":
				a.root += "/other"
			case "generation":
				s.generation++
			case "session":
				s.sessionID += "-other"
			}
			a.drain()
			if s.requestInFlight() || s.modelOverride != "" || a.agent.modelOverride != "" {
				t.Fatal("stale model callback changed a conversation")
			}
		})
	}
}

func TestAgentModelPillLabelKeepsModelName(t *testing.T) {
	for _, tc := range []struct {
		choice   acp.ModelChoice
		provider string
		want     string
	}{
		{acp.ModelChoice{ID: "magpie/deepseek", Name: "magpie/DeepSeek-V4.1-Flash"}, "pi", "DeepSeek-V4.1-Flash"},
		{acp.ModelChoice{ID: "deepseek", Name: "magpie/DeepSeek-V4.1-Flash", Group: "magpie"}, "pi", "DeepSeek-V4.1-Flash"},
		{acp.ModelChoice{ID: "magpie/deepseek"}, "pi", "deepseek"},
		{acp.ModelChoice{ID: "vendor/model", Name: "Vision/Text 2"}, "pi", "Vision/Text 2"},
		{acp.ModelChoice{ID: "local", Name: "Reason/Act"}, "pi", "Reason/Act"},
		{acp.ModelChoice{ID: "local", Name: "Alpha"}, "pi", "Alpha"},
	} {
		if got := agentModelPillLabel(tc.choice, tc.provider); got != tc.want {
			t.Errorf("pill for %+v = %q, want %q", tc.choice, got, tc.want)
		}
	}
}

func TestAgentModelPillCompactAndStable(t *testing.T) {
	hs, _ := agentRoleHarness(t, "acp")
	connectAgentModelTest(t, hs)
	s := hs.a.agent
	s.models = []acp.ModelChoice{{ID: s.model(), Name: "magpie/DeepSeek-V4.1-Flash", Group: "magpie"}}
	hs.settle()
	before, found := hs.tt.Find(locales.AgentModel())
	if !found || before.W > 196 || !hs.tt.HasText("DeepSeek-V4.1-Flash") || hs.tt.HasText("magpie/DeepSeek-V4.1-Flash") {
		t.Fatalf("model pill is not compact or contains its provider prefix: %+v", before)
	}
	hs.tt.Move(before.X+before.W/2, before.Y+before.H/2)
	hs.settle()
	after, _ := hs.tt.Find(locales.AgentModel())
	if before != after {
		t.Fatalf("hover moved the pill: before=%+v after=%+v", before, after)
	}
}
