package main

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"fiveleaf/internal/plugin"
	"fiveleaf/locales"
)

// The plugins installed from Git are the folder's own: with no account,
// there are no records of them to bring from another machine.

// pluginUpdateEvery is how often the plugins installed from Git are
// checked for new commits.
const pluginUpdateEvery = time.Hour

// checkPluginUpdates looks for new commits of the plugins installed from
// Git, from now on, and tells of those it finds once.
func (h *pluginHost) checkPluginUpdates() {
	go func() {
		defer recoverCrash()
		timer := time.NewTimer(30 * time.Second)
		for range timer.C {
			h.findPluginUpdates()
			timer.Reset(pluginUpdateEvery)
		}
	}()
}

func (h *pluginHost) findPluginUpdates() {
	recs := plugin.ReadRecords(h.reg.Dirs.Plugins)
	found := map[string]string{}
	releases := map[string]string{}
	catalog := sync.OnceValues(func() ([]plugin.IndexEntry, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return plugin.FetchIndex(ctx, nil)
	})
	for name, src := range recs {
		if src.Pinned() {
			continue
		}
		if src.TrackRelease {
			if index, err := catalog(); err == nil {
				if entry, ok := plugin.FindIndex(index, name); ok {
					if release, ok := src.NewerRelease(entry); ok {
						found[name], releases[name] = release.Version, release.Version
					}
				}
			}
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		latest, err := plugin.Latest(ctx, src)
		cancel()
		if err == nil && latest != "" && latest != src.Commit {
			found[name] = latest
		}
	}
	h.mu.Lock()
	var fresh []string
	for name, c := range found {
		if h.updates[name] != c {
			fresh = append(fresh, name)
		}
	}
	h.updates = found
	h.releases = releases
	h.mu.Unlock()
	if len(fresh) == 0 {
		return
	}
	sort.Strings(fresh)
	h.onMain(func() {
		if a := h.front(); a != nil {
			a.tell(note{message: locales.PluginsUpdatesFound(strings.Join(fresh, ", ")), label: locales.CmdPluginsManage(),
				action: func() { a.host.showSettings(prefsPlugins, "") }})
		}
	})
}

// updateRelease is the version a waiting update installs, "" for one that
// follows a commit.
func (h *pluginHost) updateRelease(name string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.releases[name]
}

// updateWaits reports whether a newer commit of a plugin was found.
func (h *pluginHost) updateWaits(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.updates[name]
	return ok
}

// updated forgets the update found of a plugin, once it is updated.
func (h *pluginHost) updated(name string) {
	h.mu.Lock()
	delete(h.updates, name)
	delete(h.releases, name)
	h.mu.Unlock()
}
