package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/git"
	"fiveleaf/internal/workspace"
)

func TestIconThemeDirectoryPickerRows(t *testing.T) {
	hs, host := iconThemePlugin(t)
	a := hs.a
	a.settings.IconTheme = "icon-test.bright"
	dir := host.reg.Plugin("icon-test").Dir
	// Only the named source icons have these color pairs. A generic folder
	// fallback would render blue and fail the same pixel check.
	blue := strings.NewReplacer("#ff0000", "#0000ff", "#00ff00", "#0000ff").Replace(testIconSVG)
	for _, name := range []string{"folder", "open"} {
		if err := os.WriteFile(filepath.Join(dir, "icons", name+".svg"), []byte(blue), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	open := strings.NewReplacer("#ff0000", "#ffff00", "#00ff00", "#ff00ff").Replace(testIconSVG)
	if err := os.WriteFile(filepath.Join(dir, "icons", "source-open.svg"), []byte(open), 0o644); err != nil {
		t.Fatal(err)
	}
	host.reg.Reload("icon-test")
	red, green := color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}
	yellow, magenta := color.RGBA{R: 255, G: 255, A: 255}, color.RGBA{R: 255, B: 255, A: 255}
	for _, tc := range []struct {
		name string
		view func(*ui.Context)
		pair [2]color.RGBA
	}{
		{"remote closed directory", func(c *ui.Context) {
			a.remotePick.query, a.remotePick.dir = "/workspace/", "/workspace/"
			a.remotePick.sel = -1
			a.pickFolderRows(c, []string{"src"}, func(int) {})
		}, [2]color.RGBA{red, green}},
		{"remote open directory", func(c *ui.Context) {
			a.remotePick.query, a.remotePick.dir = "/workspace/src", "/workspace/"
			a.remotePick.sel = -1
			a.pickFolderRows(c, []string{rowOpen}, func(int) {})
		}, [2]color.RGBA{yellow, magenta}},
		{"recent directory", func(c *ui.Context) {
			a.recents = recentPicker{sel: -1, dirs: []string{"/workspace/src"}, names: []string{"/workspace/src"}, results: []workspace.Match{{Index: 0}}}
			a.recentRow(c, 0)
		}, [2]color.RGBA{red, green}},
		{"repository directory", func(c *ui.Context) {
			a.git.repo = &git.Repo{Root: "/workspace/src"}
			a.repositories.picker = repositoryPicker{open: true}
			a.repositoryPickerView(c)
		}, [2]color.RGBA{red, green}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt := ui.NewTester(tc.view, 560, 360)
			tt.Frame()
			tt.Advance(motionEnter)
			tt.Frame()
			assertIconColorPair(t, tt.Image(), tc.pair)
		})
	}
}

func assertIconColorPair(t *testing.T, image *image.RGBA, pair [2]color.RGBA) {
	t.Helper()
	var pixels [2]int
	for y := image.Bounds().Min.Y; y < image.Bounds().Max.Y; y++ {
		for x := image.Bounds().Min.X; x < image.Bounds().Max.X; x++ {
			got := image.RGBAAt(x, y)
			for i, want := range pair {
				if got == want {
					pixels[i]++
				}
			}
		}
	}
	if pixels[0] < 30 || pixels[1] < 30 {
		t.Fatalf("directory row lost its named multicolor icon: pixels=%v colors=%v", pixels, pair)
	}
}
