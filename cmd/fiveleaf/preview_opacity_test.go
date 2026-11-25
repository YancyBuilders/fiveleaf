package main

import (
	"image"
	"os"
	"testing"

	"fiveleaf/locales"
)

func TestPreviewBackgroundOpacity(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		for _, surface := range []string{"markdown", "markdown-empty", "diff", "diff-empty", "merge", "locked-empty", "loading", "load-failed", "image", "svg"} {
			t.Run(appearance+"/"+surface, func(t *testing.T) {
				harness := newHarness(t, appearance == "dark")
				harness.a.settings.WindowOpacity = 0.6
				harness.tt.SetVibrancy(true)
				var points []image.Point
				switch surface {
				case "locked-empty":
					harness.a.openFile(harness.path("main.go"), true)
					column := harness.a.splitColumn(harness.a.column(), true)
					harness.a.toggleColumnLock(column)
					harness.settle()
					if !harness.tt.HasText(locales.LockedColumn()) {
						t.Fatal("empty locked column is missing")
					}
					points = []image.Point{{X: 1100, Y: 600}}
				case "loading", "load-failed":
					harness.a.openFile(harness.path("main.go"), true)
					harness.settle()
					editor := harness.a.activeEditor()
					editor.sync = syncLoading
					if surface == "load-failed" {
						editor.sync, editor.loadErr = syncLoadFailed, os.ErrPermission
					}
					harness.settle()
					if !editor.unread() {
						t.Fatal("unread file state was not retained")
					}
					points = []image.Point{{X: 1100, Y: 600}}
				case "image", "svg":
					path := harness.path("opacity.png")
					if surface == "svg" {
						path = harness.path("opacity.svg")
						if err := os.WriteFile(path, []byte(testSVG), 0o644); err != nil {
							t.Fatal(err)
						}
					} else {
						writePNG(t, path, 40, 30)
					}
					harness.a.openFile(path, true)
					harness.settle()
					if surface == "svg" {
						harness.a.run("view.togglePreview")
					}
					harness.until("image preview", func() bool {
						viewer := harness.a.activeViewer()
						return viewer != nil && viewer.src != nil
					})
					points = []image.Point{{X: 1100, Y: 600}}
				case "markdown", "markdown-empty":
					text := "# Preview\n\nReadable text.\n"
					if surface == "markdown-empty" {
						text = ""
					}
					if err := os.WriteFile(harness.path("opacity.md"), []byte(text), 0o644); err != nil {
						t.Fatal(err)
					}
					harness.a.openFile(harness.path("opacity.md"), true)
					harness.settle()
					if preview := harness.a.activeEditor().preview; preview != nil {
						t.Fatal("Markdown rendered a read-only view")
					}
					points = []image.Point{{X: 1100, Y: 600}}
				case "diff", "diff-empty":
					git := harness.gitRepo()
					if surface == "diff" {
						if err := os.WriteFile(harness.path("main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
							t.Fatal(err)
						}
						git("add", "main.go")
						git("commit", "-qm", "base")
						if err := os.WriteFile(harness.path("main.go"), []byte("package main\n\nfunc main() { println(1) }\n"), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					harness.settleGit()
					harness.a.showChanges(diffTarget{base: diffIndex}, true)
					if tab := harness.a.activeTab(); tab == nil || tab.diff == nil {
						t.Fatal("diff view is missing")
					} else if surface == "diff" {
						harness.settleDiff(tab.diff)
					} else {
						harness.settle()
						if len(tab.diff.files) != 0 || harness.a.git.status == nil {
							t.Fatal("clean diff view did not load")
						}
					}
					points = []image.Point{{X: 1100, Y: 600}}
				case "merge":
					harness.mergeRepo("a\nbase\nz\n", "a\nours\nz\n", "a\ntheirs\nz\n")
					merge := harness.mergeOf()
					labels := []string{merge.oursLabel(), merge.resultLabel(), merge.theirsLabel()}
					for pane := range merge.panes {
						bounds, found := harness.tt.Find(labels[pane])
						if !found {
							t.Fatalf("merge pane %d is missing", pane)
						}
						points = append(points, image.Point{X: int(bounds.X + bounds.W/2), Y: 600})
						if pane < len(merge.panes)-1 {
							points = append(points, image.Point{X: int(bounds.X + bounds.W + mergeMidW/2), Y: 600})
						}
					}
				}
				harness.settle()
				for _, fallback := range []string{"translucent", "no-material", "full-opacity"} {
					if fallback == "no-material" {
						harness.tt.SetVibrancy(false)
					} else if fallback == "full-opacity" {
						harness.tt.SetVibrancy(true)
						harness.a.settings.WindowOpacity = 1
					}
					harness.settle()
					rendered := harness.tt.Image()
					for _, point := range points {
						if !point.In(rendered.Bounds()) {
							t.Fatalf("sample %v is outside the rendered view %v", point, rendered.Bounds())
						}
						alpha := rendered.RGBAAt(point.X, point.Y).A
						if fallback == "translucent" {
							if alpha < 150 || alpha > 156 {
								t.Errorf("background at %v has alpha %d, want about 153", point, alpha)
							}
						} else if alpha != 255 {
							t.Errorf("%s background at %v has alpha %d, want 255", fallback, point, alpha)
						}
					}
				}
			})
		}
	}
}
