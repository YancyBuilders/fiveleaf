package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestClickSelectedTaskTabReturnsKeyboard(t *testing.T) {
	taskPTY(t)
	for _, source := range []string{"task", "panel"} {
		t.Run(source, func(t *testing.T) {
			hs := taskTermHarness(t)
			task := hs.a.launchTask(hs.root, waitingTask(hs.root, "Interactive task"))
			if task == nil || task.tab == nil {
				t.Fatal(hs.a.errors)
			}
			hs.until("task ready", func() bool { return strings.Contains(task.term.Text(), "task-ready") })
			if source == "panel" {
				hs.a.run("view.terminal")
				hs.settle()
				if panel := hs.a.panelTerm; panel == nil || !panel.view.Focused(hs.a.review.ctx) {
					t.Fatal("expected the panel shell to have the keyboard")
				}
			}
			if hs.a.activeTab() != task.tab {
				t.Fatal("expected the task tab to be selected before clicking it")
			}
			if err := hs.tt.Click(task.tab.name()); err != nil {
				t.Fatal(err)
			}
			hs.settle()
			if !task.view.Focused(hs.a.review.ctx) {
				t.Error("clicking the selected task tab left its terminal without keyboard focus")
			}
			hs.tt.Type("from-tab")
			hs.key(0, ui.KeyEnter)
			hs.until("task reads the keys typed after the click", func() bool {
				return strings.Contains(task.term.Text(), "answer=<from-tab>")
			})
		})
	}
}
