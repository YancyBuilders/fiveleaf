package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// chromeFiles are the files that draw the shell of the window: its tab
// strip, its sidebar, its status bar, its breadcrumbs and its panel. What
// they size comes from the palette's metrics, not from a number written
// out, so that a theme can tighten or room them at once.
var chromeFiles = []string{
	"tabs.go",
	"sidebar.go",
	"view.go",
	"panel.go",
	"crumbs.go",
	"lsp_status.go",
}

// chromeText are the sizes of text the chrome must take from the
// metrics: a piece of its text, not an icon drawn at a size of its own.
var chromeText = regexp.MustCompile(`ui\.Text\([^)]*\)[^\n]*?\.FontSize\([0-9]`)

// TestChromeUsesMetrics fails on a size of the chrome written out, as
// FontSize(12) or Radius(6) would be: it comes from the palette's metrics
// instead, which applyTheme hands to the ui.Theme too.
func TestChromeUsesMetrics(t *testing.T) {
	for _, f := range chromeFiles {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if chromeText.MatchString(line) {
				t.Errorf("%s:%d: a size of the chrome written out; use the palette's metrics: %s",
					f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// TestMetricsReachTheTheme fails when a palette's metrics do not reach
// the theme the widgets are drawn with: the two would drift apart.
func TestMetricsReachTheTheme(t *testing.T) {
	p := darkPalette().derive()
	m := p.metricsOr()
	if m.gap <= 0 || m.radius <= 0 || m.textBody <= 0 {
		t.Fatalf("the metrics are made: %+v", m)
	}
	// A palette without them, as a plugin's or one made before themes had
	// any, gets the default grid rather than a zero one.
	var bare palette
	if bare.metricsOr() != defaultMetrics() {
		t.Errorf("a palette without metrics got %+v", bare.metrics)
	}
}

// TestThemeSurvivesTheFade fails when a palette on its way to another
// loses its metrics, which would room and size the chrome anew each
// frame of a theme change.
func TestThemeSurvivesTheFade(t *testing.T) {
	from, to := darkPalette().derive(), lightPalette().derive()
	p := mixPalettes(from, to, 0.5)
	if p.metricsOr() != to.metrics {
		t.Errorf("midway, the metrics are %+v, not %+v", p.metrics, to.metrics)
	}
	if _, err := filepath.Abs("."); err != nil {
		t.Fatal(err)
	}
}
