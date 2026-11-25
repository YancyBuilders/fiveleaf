package main

import (
	"os"
	"strings"
	"testing"

	"fiveleaf/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAgentPrimarySelectionAcrossSplit(t *testing.T) {
	hs, ed := editHarness(t, "context.go", "left + selected + right\n")
	agentHome(t, hs.root)
	a := hs.a
	source := a.activeTab()
	agentViewFixture(hs)
	a.openAgentPage()
	hs.settle()
	first := a.agent
	a.splitRight()
	other := a.agent
	if other == first || other.provider != first.provider {
		t.Fatal("expected two separate chats with the same agent")
	}
	hs.settle()
	col, index := a.locate(source)
	a.activate(col, index, true)
	hs.place(ed, len("left + "), len("left + selected"))
	want := agentContextText(ed.path, 1, 1, "selected")
	a.run("agent.addSelection")
	hs.settle()
	if other.draft != want || first.draft != "" {
		t.Fatalf("the selection command used the wrong draft: other=%q first=%q", other.draft, first.draft)
	}
	a.openChatPage(first)
	hs.settle()
	a.openChatPage(other)
	hs.settle()
	bounds := first.input.Bounds(a.review.ctx)
	hs.tt.Press(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
	hs.tt.Release(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
	hs.settle()
	if a.agent != first {
		t.Fatal("clicking a split did not make it the active chat")
	}
	a.activate(col, index, true)
	hs.place(ed, len("left + "), len("left + selected"))
	box, found := hs.tt.Find(locales.EditorLabel(ed.name()))
	if !found {
		t.Fatal("the split editor is missing")
	}
	hs.tt.RightClickAt(box.X+ed.gutterW+gutterGap+8*a.font.charW, box.Y+editorPadY+a.font.lineH/2)
	if err := hs.tt.ChooseMenuItem(locales.CmdAgentAddSelection()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if first.draft != want || other.draft != want || a.agent != first || !first.input.Focused(a.review.ctx) {
		t.Fatalf("the editor did not open the last focused draft: first=%q other=%q", first.draft, other.draft)
	}
	for _, chat := range []*agentChat{first, other} {
		if len(chat.messages) != 0 || chat.client != nil || chat.busy || chat.connecting {
			t.Fatal("adding the selection sent it without a send action")
		}
	}
}

func TestAgentPrimarySelectionShortcutFollowsKeyboardFocus(t *testing.T) {
	hs, ed := editHarness(t, "context.go", "first()\nsecond()\n")
	agentHome(t, hs.root)
	a := hs.a
	source := a.activeTab()
	agentViewFixture(hs)
	hs.settle()
	first := a.agent
	a.openAgentPage()
	a.splitRight()
	hs.settle()
	second := a.agent
	item := a.menu(false, func(id string, _ *mygo.Window) { a.run(id) }).ItemByID("agent.addSelection")
	if item == nil || item.Click == nil {
		t.Fatal("the native Add Selection command is missing")
	}
	if stroke, ok := parseAccel(item.Accelerator); !ok || stroke.Mods != ui.Cmd|ui.Alt || stroke.Key != ui.KeyL {
		t.Fatalf("unexpected macOS shortcut: %q", item.Accelerator)
	}
	addFromEditor := func(start, end int) {
		t.Helper()
		col, index := a.locate(source)
		a.activate(col, index, true)
		hs.place(ed, start, end)
		item.Click(item, nil)
		hs.settle()
	}
	addFromEditor(0, len("first()"))
	want := agentContextText(ed.path, 1, 1, "first()")
	if second.draft != want || first.draft != "" || a.agent != second {
		t.Fatal("opening the second split did not redirect the selection shortcut")
	}
	a.openChatPage(first)
	a.openChatPage(second)
	hs.settle()
	a.run("view.prevColumn")
	hs.settle()
	if a.agent != first || !first.input.Focused(a.review.ctx) {
		t.Fatal("focusing a split with the keyboard did not update the selection target")
	}
	addFromEditor(len("first()\n"), len("first()\nsecond()"))
	if second.draft != want || first.draft != agentContextText(ed.path, 2, 2, "second()") || a.agent != first {
		t.Fatal("the shortcut did not follow the newly focused split")
	}
}

func TestAgentPrimarySelectionLifecycle(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.openAgentPage()
	hs.settle()
	first := a.agent
	a.splitRight()
	sibling := a.agent
	a.openChatPage(first)
	slot := a.activeTab()
	a.settings.AgentServers["other"] = a.settings.AgentServers[first.provider]
	a.selectChatAgent(first, "other")
	if a.agent != slot.tool.chat || a.agent == first {
		t.Fatal("switching providers left a hidden draft as the target")
	}
	a.closeTab(slot, nil, nil)
	if a.agent != sibling {
		t.Fatal("closing the last active split did not target its sibling")
	}
	a.reopenClosed()
	hs.settle()
	if a.agent != slot.tool.chat {
		t.Fatal("reopening a chat did not make it the current target")
	}
	before := a.agent
	a.openFolder(workspaceFixture(t))
	if a.agent == before || a.agent == sibling || a.agent != a.activeTab().tool.chat {
		t.Fatal("the new workspace inherited another project's draft")
	}
}

func TestAgentPrimarySelectionResetsWithWorkspaceBehindEditor(t *testing.T) {
	hs := agentWorkspaceHarness(t)
	a := hs.a
	a.newChatPage()
	old, slot := a.agent, a.activeTab()
	old.draft = "previous workspace draft"
	a.openFile(hs.path("main.go"), true)
	a.openFolder(workspaceFixture(t))
	if a.agent == old || a.agent != slot.tool.chat || a.agent.draft != "" || a.agent.connecting || a.agent.client != nil {
		t.Fatal("changing projects behind an editor did not reset the last selected chat in place")
	}
}

func TestAgentPageContextSelectionFromHiddenEditor(t *testing.T) {
	hs, d := editHarness(t, "context.go", "first()\nsecond()\n")
	hs.place(d, 0, len("first()"))
	a := hs.a
	source := a.activeTab()
	a.openAgentPage()
	hs.settle()
	page := a.activeTab()
	a.run("agent.addSelection")
	hs.settle()
	if a.agentContextTab() != source || a.activeTab() != page || !strings.Contains(a.agent.draft, "first()") ||
		strings.Contains(a.agent.draft, "second()") || d.sel != [2]int{0, len("first()")} {
		t.Fatalf("the page did not retain its source selection: draft=%q selection=%v", a.agent.draft, d.sel)
	}
	before := a.agent.draft
	hs.tt.Type(" Continue here.")
	hs.settle()
	if !strings.Contains(a.agent.draft, " Continue here.") || !strings.Contains(a.agent.draft, before) ||
		a.agent.client != nil || a.agent.connecting || len(a.agent.messages) != 0 {
		t.Fatal("adding a selection did not leave an editable, unsent draft in the page")
	}
}

func TestAgentPageContextCurrentFileUsesUnsavedBuffer(t *testing.T) {
	const disk = "saved()\n"
	const unsaved = "changedOnlyInEditor()\n"
	hs, d := editHarness(t, "context.go", disk)
	a := hs.a
	a.settings.AutoSave = false
	d.edit([2]int{0, len(unsaved)}, textEdit{start: 0, end: len(disk), text: unsaved})
	hs.settle()
	if !d.dirty() {
		t.Fatal("the fixture has no unsaved edit")
	}
	a.openAgentPage()
	const question = "解释这里的改动"
	hs.settle()
	hs.tt.Type(question)
	hs.settle()
	hs.key(ui.Shift, ui.KeyLeft)
	selection := a.agent.draftState.selection
	if selection[0] == selection[1] {
		t.Fatal("the fixture has no selected draft text")
	}
	page := a.activeTab()
	a.run("agent.addFile")
	hs.settle()
	a.run("agent.addFile")
	hs.settle()
	want := question + "\n\n" + agentContextText(d.path, 0, 0, unsaved)
	if a.activeTab() != page || a.agent.draft != question || a.agent.draftPrompt() != want {
		t.Fatalf("attaching again changed the draft or its snapshot: %q, %q", a.agent.draft, a.agent.draftPrompt())
	}
	if a.agent.draftState.selection != selection {
		t.Fatalf("attaching changed the text selection from %v to %v", selection, a.agent.draftState.selection)
	}
	if text, err := os.ReadFile(d.path); err != nil || string(text) != disk {
		t.Fatalf("adding context saved the file: %q, %v", text, err)
	}
	if err := hs.tt.Click(locales.AgentChatRemoveAttachment("context.go")); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.agent.draft != question || a.agent.draftPrompt() != question || len(a.agent.detailSources()) != 0 {
		t.Fatal("removing the file changed the question or left the attachment in the prompt")
	}
}

func TestAgentPageContextTracksMouseFocusAcrossColumns(t *testing.T) {
	hs, left := editHarness(t, "context.go", "alpha()\nbeta()\n")
	a := hs.a
	leftTab := a.activeTab()
	a.splitRight()
	hs.settle()
	right := a.activeEditor()
	hs.place(right, len("alpha()\n"), len("alpha()\nbeta()"))
	a.openAgentPage()
	hs.settle()
	page := a.activeTab()
	box, ok := hs.tt.Find(locales.EditorLabel(left.name()))
	if !ok {
		t.Fatal("the other column's editor is not visible")
	}
	x := box.X + left.gutterW + gutterGap
	y := box.Y + editorPadY + a.font.lineH/2
	hs.tt.Press(x+0.2*a.font.charW, y)
	hs.tt.Move(x+float32(len("alpha()"))*a.font.charW+0.2*a.font.charW, y)
	hs.tt.Release(x+float32(len("alpha()"))*a.font.charW+0.2*a.font.charW, y)
	hs.settle()
	if left.sel != [2]int{0, len("alpha()")} {
		t.Fatalf("the mouse fixture did not select the first line: %v", left.sel)
	}
	if err := hs.tt.Click(locales.AgentDraft()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if a.activeTab() != page || a.agentContextTab() != leftTab {
		t.Fatal("clicking the composer forgot the last editor used in another column")
	}
	a.run("agent.addSelection")
	hs.settle()
	if !strings.Contains(a.agent.draft, "alpha()") || strings.Contains(a.agent.draft, "beta()") ||
		right.sel != [2]int{len("alpha()\n"), len("alpha()\nbeta()")} {
		t.Fatalf("context used the wrong view's selection: draft=%q right=%v", a.agent.draft, right.sel)
	}
}

func TestAgentPageContextKeepsBackgroundMarkdownPreview(t *testing.T) {
	hs, d := editHarness(t, "context.md", "# Heading\n\nKeep this paragraph.\n")
	hs.place(d, 0, len("# Heading"))
	a := hs.a
	a.setPreview(d, true)
	hs.settle()
	preview := d.preview
	a.openAgentPage()
	hs.settle()
	page := a.activeTab()
	a.run("agent.addSelection")
	hs.settle()
	if preview == nil || d.preview != preview || a.activeTab() != page || !strings.Contains(a.agent.draft, "# Heading") {
		t.Fatal("adding context changed the hidden Markdown preview or left the Agent page")
	}
}

func TestAgentPageContextKeepsDiffSelection(t *testing.T) {
	hs, v, path, _ := diffSelTab(t, layoutInline)
	a := hs.a
	from, to := diffOldRow(v, path, 25), diffEditRow(v, path, 26)
	if from < 0 || to < 0 {
		t.Fatal("the diff fixture has no changed rows")
	}
	v.setCursor(to)
	v.anchor = from
	hs.settle()
	lo, hi := v.selection()
	want := formatReview(v.excerpts(lo, hi), a.relPath)
	if want == "" {
		t.Fatal("the diff fixture has no selected excerpt")
	}
	a.openAgentPage()
	hs.settle()
	page := a.activeTab()
	a.run("agent.addSelection")
	hs.settle()
	if a.activeTab() != page || a.agent.draft != want {
		t.Fatalf("adding the hidden diff selection changed its excerpt: %q, want %q", a.agent.draft, want)
	}
}

func TestAgentPageContextRejectsClosedOrOtherWorkspaceSource(t *testing.T) {
	for _, change := range []string{"close", "workspace"} {
		t.Run(change, func(t *testing.T) {
			hs, _ := editHarness(t, "context.go", "oldWorkspace()\n")
			a := hs.a
			source := a.activeTab()
			a.openAgentPage()
			hs.settle()
			if change == "close" {
				a.closeTab(source, nil, nil)
			} else {
				a.openFolder(workspaceFixture(t))
			}
			hs.settle()
			if a.agentContextTab() != nil {
				t.Fatal("context still refers to a closed tab or another workspace")
			}
			a.run("agent.addSelection")
			a.run("agent.addFile")
			hs.settle()
			if a.agent.draft != "" {
				t.Fatalf("unavailable context changed the draft: %q", a.agent.draft)
			}
		})
	}
}
