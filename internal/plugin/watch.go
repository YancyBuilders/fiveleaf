package plugin

import (
	_ "embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TypesSource is fiveleaf.d.ts, the types of the fiveleaf module.
//
//go:embed fiveleaf.d.ts
var TypesSource string

// AgentsSource is the AGENTS.md ff plugin new writes, for agents that
// write plugins.
//
//go:embed agents.md
var AgentsSource string

// watchDirs watches the plugins, scripts and themes folders, and the
// folders of the plugins in them, symlinks followed, and loads them again
// once changes rest for delay. A change of the themes alone tells the host
// of it without loading the plugins, whose code did not change.
func watchDirs(r *Registry, delay time.Duration) (func(), error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	watched := map[string]bool{}
	add := func(dir string) {
		mu.Lock()
		defer mu.Unlock()
		if dir == "" || watched[dir] {
			return
		}
		if w.Add(dir) == nil {
			watched[dir] = true
		}
	}
	// addTree watches a plugin's folders, but those of its packages.
	addTree := func(dir string) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			add(p)
			return nil
		})
	}
	rewatch := func() {
		for _, dir := range []string{r.Dirs.Plugins, r.Dirs.Scripts, r.Dirs.Themes} {
			if dir == "" {
				continue
			}
			os.MkdirAll(dir, 0o755)
			add(dir)
		}
		if r.Dirs.Plugins != "" {
			entries, _ := os.ReadDir(r.Dirs.Plugins)
			for _, e := range entries {
				addTree(filepath.Join(r.Dirs.Plugins, e.Name()))
			}
		}
	}
	rewatch()
	themesChanged := func() {
		if r.host != nil {
			r.host.Changed()
		}
	}
	done := make(chan struct{})
	go func() {
		var timer *time.Timer
		var themeOnly bool
		var pending bool
		fire := make(chan struct{}, 1)
		// after waits for the changes to rest, and then asks for one
		// reload: a change of the themes alone asks for that alone.
		after := func() {
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(delay, func() {
				select {
				case fire <- struct{}{}:
				default:
				}
			})
		}
		for {
			select {
			case <-done:
				if timer != nil {
					timer.Stop()
				}
				return
			case ev, ok := <-w.Events:
				if !ok {
					if timer != nil {
						timer.Stop()
					}
					return
				}
				if b := filepath.Base(ev.Name); b == RecordsFile || strings.HasPrefix(b, ".install-") || strings.HasSuffix(b, ".old") {
					continue
				}
				inThemes := inDir(r.Dirs.Themes, ev.Name)
				if !pending && inThemes {
					themeOnly = true
				} else if !inThemes {
					themeOnly = false
				}
				pending = true
				after()
			case <-fire:
				timer = nil
				if pending && themeOnly {
					themesChanged()
				} else if pending {
					rewatch()
					r.Load()
				}
				pending, themeOnly = false, false
			case _, ok := <-w.Errors:
				if !ok {
					if timer != nil {
						timer.Stop()
					}
					return
				}
			}
		}
	}()
	return func() {
		close(done)
		w.Close()
	}, nil
}

// inDir reports whether name is in dir or a folder below it.
func inDir(dir, name string) bool {
	if dir == "" {
		return false
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if real, err := filepath.EvalSymlinks(name); err == nil {
		name = real
	}
	rel, err := filepath.Rel(dir, name)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
