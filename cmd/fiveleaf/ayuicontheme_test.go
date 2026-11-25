package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAyuIconThemeAssets(t *testing.T) {
	withAppearancePlugins(t)
	pack := iconThemeByID("ayu")
	if pack == nil {
		t.Fatal("the installed icon plugin did not provide Ayu")
	}
	if !slices.Contains(allIconThemes(), iconThemeChoice{"fiveleaf-icons.ayu", "Ayu"}) {
		t.Fatal("Ayu is missing from the shared icon theme choices")
	}
	for _, tc := range []struct{ name, icon string }{
		{"main.go", "go"}, {"go.mod", "gomod"}, {"module.d.ts", "typescriptdef"},
		{"App.tsx", "tsx"}, {"component.jsx", "jsx"}, {"Cargo.toml", "cargo"},
		{"package.json", "npm"}, {"Dockerfile", "docker"}, {"README.md", "markdown"},
		{".gitignore", "git"}, {".env", "dotenv"}, {"image.png", "image"}, {"unknown", "_file"},
	} {
		if got, _ := fileIconForTheme(darkPal, "ayu", tc.name); got != pack.icons[tc.icon] {
			t.Errorf("%s did not select Ayu's %s icon", tc.name, tc.icon)
		}
	}
	closed, _ := folderIconForTheme(darkPal, "ayu", "src", false)
	open, _ := folderIconForTheme(darkPal, "ayu", "src", true)
	if closed != pack.icons["_folder_dark"] || open != pack.icons["_folder_open_dark"] {
		t.Error("Ayu did not use its standard collapsed and expanded folder icons")
	}
	// Validate the renderer's SVG subset as well as the loader: a valid SVG
	// using an unsupported element must not silently leave an empty slot.
	for id, icon := range pack.icons {
		t.Run(id, func(t *testing.T) {
			background := darkPal.editor
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Size(32, 32).Background(background).Children(func() {
					iconThemeView(c, darkPal, icon, white, 32)
				})
			}, 32, 32)
			tt.Frame()
			img, painted := tt.Image(), 0
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					pixel := img.RGBAAt(x, y)
					if pixel.R != background.R || pixel.G != background.G || pixel.B != background.B {
						painted++
					}
				}
			}
			if painted < 20 {
				t.Errorf("Ayu icon %s painted only %d pixels", id, painted)
			}
		})
	}
}

func TestAyuIconThemePNGRendering(t *testing.T) {
	withAppearancePlugins(t)
	pack := iconThemeByID("ayu")
	if pack == nil {
		t.Fatal("Ayu is unavailable")
	}
	for _, tc := range []struct{ icon, original string }{
		{"_folder_dark", "folder_dark@2x.png"},
		{"_folder_open_dark", "folder_opened_dark@2x.png"},
		{"docker", "file_type_docker@2x.png"},
		{"editorconfig", "file_type_editorconfig@2x.png"},
		{"jsx", "file_type_jsx@2x.png"},
		{"lock", "file_type_lock@2x.png"},
		{"markup", "file_type_markup@2x.png"},
		{"rails", "file_type_rails@2x.png"},
		{"sql", "file_type_sql@2x.png"},
		{"yarn", "file_type_yarn@2x.png"},
	} {
		t.Run(tc.icon, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", "ayu-icons", tc.original))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			original, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []*palette{lightPal, darkPal} {
				background := p.editor
				want := image.NewRGBA(original.Bounds())
				draw.Draw(want, want.Bounds(), image.NewUniform(color.RGBA{background.R, background.G, background.B, 255}), image.Point{}, draw.Src)
				draw.Draw(want, want.Bounds(), original, image.Point{}, draw.Over)
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Box(c).Size(32, 32).Background(background).Children(func() {
						iconThemeView(c, p, pack.icons[tc.icon], white, 32)
					})
				}, 32, 32)
				tt.Frame()
				got := tt.Image()
				for y := 0; y < 32; y++ {
					for x := 0; x < 32; x++ {
						a, b := got.RGBAAt(x, y), want.RGBAAt(x, y)
						for channel, pair := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
							if delta := max(pair[0], pair[1]) - min(pair[0], pair[1]); delta > 2 {
								t.Fatalf("dark=%v pixel (%d,%d) channel %d: got %v, want original PNG %v", p.dark, x, y, channel, a, b)
							}
						}
					}
				}
			}
		})
	}
}

func TestAyuIconThemePicker(t *testing.T) {
	withAppearancePlugins(t)
	hs := newHarness(t, true)
	a := hs.a
	a.settings.IconTheme = "material"
	a.run("view.selectIconTheme")
	hs.settle()
	hs.tt.Type("Ayu")
	hs.settle()
	expectIconThemePreview(t, a, "fiveleaf-icons.ayu")
	if a.settings.IconTheme != "material" {
		t.Fatal("Ayu preview changed the saved icon theme")
	}
	hs.shot("ayu-icon-picker")
	hs.key(0, ui.KeyEscape)
	if a.settings.IconTheme != "material" {
		t.Fatal("canceling Ayu lost the saved icon theme")
	}
	a.run("view.selectIconTheme")
	hs.settle()
	hs.tt.Type("Ayu")
	hs.settle()
	hs.key(0, ui.KeyEnter)
	if a.settings.IconTheme != "fiveleaf-icons.ayu" || a.iconPicker.open {
		t.Fatal("confirming Ayu did not keep the selected icon theme")
	}
	icon, _ := a.fileIcon("main.go")
	if icon != iconThemeByID("ayu").icons["go"] {
		t.Fatal("the editor did not retain Ayu's icons after closing the picker")
	}
}

func TestShotAyuIcons(t *testing.T) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		t.Skip()
	}
	withAppearancePlugins(t)
	pack := iconThemeByID("ayu")
	if pack == nil {
		t.Fatal("Ayu is unavailable")
	}
	ids := make([]string, 0, len(pack.icons))
	for id := range pack.icons {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(0).Children(func() {
			for _, p := range []*palette{lightPal, darkPal} {
				ui.Column(c).Width(560).Height(688).Padding(16).Gap(8).Background(p.editor).Children(func() {
					label := "Ayu icons · Light background"
					if p.dark {
						label = "Ayu icons · Dark background"
					}
					ui.Text(c, label).FontSize(18).TextColor(p.text)
					for row := 0; row < len(ids); row += 6 {
						ui.Row(c).Gap(8).Children(func() {
							for _, id := range ids[row:min(row+6, len(ids))] {
								ui.Column(c).Width(80).Height(48).Gap(4).AlignItems(ui.Center).Children(func() {
									iconThemeView(c, p, pack.icons[id], white, 24)
									ui.Text(c, id).FontSize(10).TextColor(p.text)
								})
							}
						})
					}
				})
			}
		})
	}, 1120, 688)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "ayu-icons-light-dark.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
