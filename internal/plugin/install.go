package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fiveleaf/extensions"
)

// RecordsFile is the file in the plugins folder where FiveLeaf notes where
// each plugin it installed came from.
const RecordsFile = "plugins.json"

// Source is where an installed plugin came from.
type Source struct {
	URL    string `json:"url"`
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Version is the release installed from the index. A source whose
	// Version is empty is a branch, a commit, or a local clone, which
	// updates by commit rather than by a release.
	Version      string `json:"version,omitempty"`
	TrackRelease bool   `json:"trackRelease,omitempty"`
}

// Pinned reports whether the installed release should stay at its version.
func (src Source) Pinned() bool { return src.Version != "" && !src.TrackRelease }

// Records are the sources of the installed plugins, by name.
type Records map[string]Source

// ReadRecords reads plugins.json of a plugins folder.
func ReadRecords(dir string) Records {
	recs, err := readRecords(dir)
	if err != nil {
		return Records{}
	}
	return recs
}

func readRecords(dir string) (Records, error) {
	file := filepath.Join(dir, RecordsFile)
	info, err := os.Lstat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return Records{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", file)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var recs Records
	if err := json.Unmarshal(data, &recs); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if recs == nil {
		return nil, fmt.Errorf("%s must contain an object", file)
	}
	for name, source := range recs {
		source.URL, err = absoluteLocalSource(dir, source.URL)
		if err != nil {
			return nil, err
		}
		recs[name] = source
	}
	return recs, nil
}

// WriteRecords writes plugins.json.
func WriteRecords(dir string, recs Records) error {
	return changePlugins(dir, func(work string, _ Records) error {
		file := filepath.Join(work, RecordsFile)
		if err := stageRecords(file, recs); err != nil {
			return err
		}
		return os.Rename(file, filepath.Join(dir, RecordsFile))
	})
}

func stageRecords(file string, recs Records) error {
	if recs == nil {
		recs = Records{}
	}
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(data, '\n'), 0o644)
}

const officialIndexURL = "https://raw.githubusercontent.com/YancyBuilders/fiveleaf-plugins/main/index.json"

// IndexURL is the official FiveLeaf marketplace index.
var IndexURL = officialIndexURL

// IndexVersion is FiveLeaf's version when the index is read outside the
// app, to skip releases the caller cannot run. It is "dev" by default,
// which satisfies every range.
var IndexVersion string

// IndexEntry is a plugin the index lists.
type IndexEntry struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description"`
	Author      string `json:"author,omitempty"`
	// Category is one of plugin.Categories.
	Category string `json:"category"`
	// Version is the release at the top of Versions.
	Version string `json:"version"`
	// FiveLeaf is the versions of FiveLeaf this release runs on.
	FiveLeaf string `json:"fiveleaf,omitempty"`
	// Repository is the Git repository, with #subdirectory for a plugin
	// in a folder of a larger repository.
	Repository string `json:"repository"`
	// Versions are the releases this entry lists, the newest first. The
	// top is what Version names, and what an install without a version
	// takes.
	Versions []VersionEntry `json:"versions,omitempty"`
	// Tags are search words, never categories.
	Tags []string `json:"tags,omitempty"`
}

// VersionEntry is one release of a plugin: its version, the Git commit
// that holds it, and the versions of FiveLeaf it runs on.
type VersionEntry struct {
	Version  string `json:"version"`
	Commit   string `json:"commit,omitempty"`
	FiveLeaf string `json:"fiveleaf,omitempty"`
	Path     string `json:"path,omitempty"`
	Ref      string `json:"ref,omitempty"`
}

type IndexNameNotFound string

func (name IndexNameNotFound) Error() string {
	return fmt.Sprintf("no plugin %s in the index: ff plugin search lists them", string(name))
}

func (entry IndexEntry) Install(ctx context.Context, dir, ref string) (*Manifest, error) {
	if err := ValidName(entry.Name); err != nil {
		return nil, err
	}
	if err := entry.check(); err != nil {
		return nil, err
	}
	release, pinned := entry.version(ref)
	var ok bool
	if pinned {
		if !release.compatible() {
			return nil, fmt.Errorf("%s %s needs FiveLeaf %s, and this is %s", entry.Name, release.Version, release.FiveLeaf, indexVersion())
		}
	} else if release, ok = entry.Newest(); !ok {
		return nil, fmt.Errorf("%s has no release for this FiveLeaf", entry.Name)
	}
	src := entry.releaseSource(release, ref == "")
	if ref != "" && !pinned {
		src.Ref, src.Commit, src.Version = ref, "", ""
	}
	manifest, _, err := installGit(ctx, dir, entry.Name, src, nil)
	return manifest, err
}

// version finds a release of the entry by its version.
func (entry IndexEntry) version(version string) (VersionEntry, bool) {
	want, ok := ParseSemver(version)
	if !ok {
		return VersionEntry{}, false
	}
	if len(entry.Versions) == 0 {
		got, ok := ParseSemver(entry.Version)
		return VersionEntry{Version: entry.Version, FiveLeaf: entry.FiveLeaf}, ok && CompareSemver(got, want) == 0
	}
	for _, release := range entry.Versions {
		got, ok := ParseSemver(release.Version)
		if ok && CompareSemver(got, want) == 0 {
			return release, true
		}
	}
	return VersionEntry{}, false
}

func (entry IndexEntry) releaseSource(release VersionEntry, track bool) Source {
	return Source{URL: entry.source(release), Ref: release.Ref, Commit: release.Commit, Version: release.Version, TrackRelease: track}
}

// source is the repository of a release, with the folder this release
// keeps the plugin in.
func (entry IndexEntry) source(release VersionEntry) string {
	repo, subdir, _ := strings.Cut(entry.Repository, "#")
	if release.Path != "" {
		subdir = release.Path
	}
	if subdir != "" {
		return repo + "#" + subdir
	}
	return repo
}

// Newest is the newest release of the entry that this FiveLeaf can run.
// An entry with no list is its own only release.
func (entry IndexEntry) Newest() (VersionEntry, bool) {
	if len(entry.Versions) == 0 {
		release := VersionEntry{Version: entry.Version, FiveLeaf: entry.FiveLeaf}
		return release, release.compatible()
	}
	for _, release := range entry.Versions {
		if release.compatible() {
			return release, true
		}
	}
	return VersionEntry{}, false
}

func indexVersion() string {
	if IndexVersion == "" {
		return "dev"
	}
	return IndexVersion
}

func (release VersionEntry) compatible() bool {
	return release.FiveLeaf == "" || Satisfies(indexVersion(), release.FiveLeaf)
}

// NewerRelease returns the compatible update for a source that follows the index.
func (src Source) NewerRelease(entry IndexEntry) (VersionEntry, bool) {
	if !src.TrackRelease || !sameRepository(src.URL, entry.Repository) {
		return VersionEntry{}, false
	}
	release, ok := entry.Newest()
	if !ok {
		return VersionEntry{}, false
	}
	current, currentOK := ParseSemver(src.Version)
	next, nextOK := ParseSemver(release.Version)
	return release, currentOK && nextOK && CompareSemver(next, current) > 0
}

// check reports whether an index entry is a release of a known category,
// with a version and an installable source.
func (entry IndexEntry) check() error {
	if _, ok := ParseSemver(entry.Version); !ok {
		return fmt.Errorf("%s: version %q is not a version such as \"1.2.3\"", entry.Name, entry.Version)
	}
	for _, release := range entry.Versions {
		if _, ok := ParseSemver(release.Version); !ok {
			return fmt.Errorf("%s: version %q is not a version such as \"1.2.3\"", entry.Name, release.Version)
		}
		if release.Path != "" && (!fs.ValidPath(release.Path) || strings.ContainsAny(release.Path, "\\:\x00")) {
			return fmt.Errorf("%s: release path %q must be inside the repository", entry.Name, release.Path)
		}
	}
	if len(entry.Versions) > 0 && entry.Versions[0].Version != entry.Version {
		return fmt.Errorf("%s: the newest release is %s, and the entry says %s", entry.Name, entry.Versions[0].Version, entry.Version)
	}
	if !ValidCategory(entry.Category) {
		return fmt.Errorf("%s: category %q is not one of %s", entry.Name, entry.Category, strings.Join(Categories, ", "))
	}
	if entry.Repository == "" {
		return fmt.Errorf("%s: the index gives no repository", entry.Name)
	}
	for _, release := range entry.Versions {
		if strings.HasPrefix(release.Ref, "-") {
			return fmt.Errorf("%s: invalid release ref %q", entry.Name, release.Ref)
		}
	}
	return nil
}

func InstallSpec(ctx context.Context, dir, spec string, index []IndexEntry) (*Manifest, error) {
	source, ref, byName := SplitSpec(spec)
	if !byName {
		return Install(ctx, dir, source, ref)
	}
	var fetchErr error
	if index == nil {
		index, fetchErr = FetchIndex(ctx, nil)
	}
	for _, entry := range index {
		if entry.Name == source {
			return entry.Install(ctx, dir, ref)
		}
	}
	if fetchErr != nil {
		return nil, fmt.Errorf("reading the index: %w", fetchErr)
	}
	return nil, IndexNameNotFound(source)
}

// FetchIndex reads the live plugin index. On a server error it returns the
// bundled entries alongside the error so callers can identify the fallback.
func FetchIndex(ctx context.Context, client *http.Client) ([]IndexEntry, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, "GET", IndexURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		statusErr := fmt.Errorf("HTTP %s", resp.Status)
		if req.URL.String() == officialIndexURL && resp.StatusCode >= 500 && resp.StatusCode < 600 {
			entries, err := decodeIndex([]byte(extensions.CatalogJSON))
			return entries, errors.Join(statusErr, err)
		}
		return nil, statusErr
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return decodeIndex(data)
}

func decodeIndex(data []byte) ([]IndexEntry, error) {
	var entries []IndexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		var wrapped struct {
			Plugins []IndexEntry `json:"plugins"`
		}
		if json.Unmarshal(data, &wrapped) != nil {
			return nil, fmt.Errorf("the index does not read: %v", err)
		}
		entries = wrapped.Plugins
	}
	if entries == nil {
		return nil, errors.New("the index does not contain a plugin list")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// SplitSpec reads what ff plugin install takes: a Git URL, owner/repo
// of GitHub, or a name of the index, with an optional #subdirectory and @ref.
func SplitSpec(spec string) (url, ref string, byName bool) {
	repo, subdir, selected := strings.Cut(spec, "#")
	if at := strings.LastIndexByte(subdir, '@'); at >= 0 {
		subdir, ref = subdir[:at], subdir[at+1:]
	} else if at := strings.LastIndexByte(repo, '@'); at > 0 {
		bareName := ValidName(repo[:at]) == nil && !strings.Contains(repo[at+1:], ":")
		if at > strings.LastIndexAny(repo, `/\:`) || bareName || strings.HasSuffix(repo[:at], ".git") {
			repo, ref = repo[:at], repo[at+1:]
		}
	}
	switch {
	case strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") || filepath.IsAbs(repo) || strings.HasPrefix(repo, "."):
	case strings.Count(repo, "/") == 1:
		repo = "https://github.com/" + repo
	default:
		byName = !selected
	}
	if selected {
		repo += "#" + subdir
	}
	return repo, ref, byName
}

func splitRepository(source string) (repo, subdir string, err error) {
	repo, subdir, selected := strings.Cut(source, "#")
	if strings.TrimSpace(repo) == "" || strings.HasPrefix(repo, "-") {
		return "", "", fmt.Errorf("invalid Git repository %q", repo)
	}
	if selected {
		if !fs.ValidPath(subdir) || subdir == "." || strings.ContainsAny(subdir, "\\:\x00#") || !filepath.IsLocal(filepath.FromSlash(subdir)) {
			return "", "", fmt.Errorf("invalid plugin subdirectory %q", subdir)
		}
		for part := range strings.SplitSeq(subdir, "/") {
			if strings.EqualFold(part, ".git") || strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") {
				return "", "", fmt.Errorf("invalid plugin subdirectory %q", subdir)
			}
		}
	}
	return repo, subdir, nil
}

func absoluteLocalSource(base, source string) (string, error) {
	repo, _, _ := strings.Cut(source, "#")
	if strings.TrimSpace(repo) == "" || filepath.IsAbs(repo) || strings.Contains(repo, ":") || strings.HasPrefix(repo, "-") {
		return source, nil
	}
	abs, err := filepath.Abs(filepath.Join(base, repo))
	if err != nil {
		return "", err
	}
	return abs + source[len(repo):], nil
}

// git runs git in dir, returning what it printed or why it failed.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// Install installs the plugin at a Git repository's root or #subdirectory.
func Install(ctx context.Context, dir, url, ref string) (*Manifest, error) {
	manifest, _, err := installGit(ctx, dir, "", Source{URL: url, Ref: ref}, nil)
	return manifest, err
}

func clonePlugin(ctx context.Context, checkout, repo, ref string) (string, error) {
	if strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("invalid Git ref %q", ref)
	}
	args := []string{"clone", "--depth", "1", "--quiet"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	if _, err := git(ctx, "", append(args, "--", repo, checkout)...); err != nil {
		if ref == "" {
			return "", err
		}
		// A commit, which --branch does not take.
		if _, err := git(ctx, "", "clone", "--quiet", "--no-checkout", "--", repo, checkout); err != nil {
			return "", err
		}
		if _, err := git(ctx, checkout, "checkout", "--quiet", "--detach", ref, "--"); err != nil {
			return "", err
		}
	}
	return git(ctx, checkout, "rev-parse", "HEAD")
}

func pluginTree(ctx context.Context, checkout, subdir string) (string, error) {
	tree := "HEAD"
	root := checkout
	if subdir != "" {
		tree += ":" + subdir
		for part := range strings.SplitSeq(subdir, "/") {
			root = filepath.Join(root, part)
			info, err := os.Lstat(root)
			if err != nil {
				return "", err
			}
			if !info.IsDir() {
				return "", fmt.Errorf("%s must be a directory, not a link or file", root)
			}
		}
	}
	entries, err := git(ctx, checkout, "ls-tree", "-rz", tree)
	if err != nil {
		return "", err
	}
	for record := range strings.SplitSeq(entries, "\x00") {
		if strings.HasPrefix(record, "120000 ") {
			_, name, _ := strings.Cut(record, "\t")
			return "", fmt.Errorf("plugin file %q is a symbolic link", name)
		}
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == filepath.Join(checkout, ".git") && entry.IsDir() {
			return fs.SkipDir
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("%s must be a regular plugin file", path)
		}
		return nil
	})
	return root, err
}

func installGit(ctx context.Context, dir, name string, src Source, previous *Source) (*Manifest, string, error) {
	var err error
	src.URL, err = absoluteLocalSource("", src.URL)
	if err != nil {
		return nil, "", err
	}
	repo, subdir, err := splitRepository(src.URL)
	if err != nil {
		return nil, "", err
	}
	if err := ensurePluginDir(dir); err != nil {
		return nil, "", err
	}
	work, err := os.MkdirTemp(dir, ".install-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(work)
	checkout := filepath.Join(work, "repo")
	checkoutRef := src.Ref
	if src.Commit != "" {
		checkoutRef = src.Commit
	}
	src.Commit, err = clonePlugin(ctx, checkout, repo, checkoutRef)
	if err != nil {
		return nil, "", err
	}
	staged, err := pluginTree(ctx, checkout, subdir)
	if err != nil {
		return nil, "", err
	}
	manifest, err := ReadManifest(staged)
	if err != nil {
		return nil, "", fmt.Errorf("%s is not a FiveLeaf plugin: %w", src.URL, err)
	}
	if name != "" && manifest.Name != name {
		return nil, "", fmt.Errorf("expected plugin %q from %s, found %q", name, src.URL, manifest.Name)
	}
	if src.Version != "" && manifest.Version != src.Version {
		return nil, "", fmt.Errorf("%s says version %s, and the index says %s", manifest.Name, manifest.Version, src.Version)
	}
	if err := checkAppearanceAssets(staged, manifest); err != nil {
		return nil, "", err
	}
	if err := checkToolboxPage(staged, manifest); err != nil {
		return nil, "", err
	}
	err = changePlugins(dir, func(transaction string, records Records) error {
		name := manifest.Name
		if previous != nil && records[name] != *previous {
			return fmt.Errorf("the installation of %s changed during checkout", name)
		}
		exists, err := checkDestination(dir, name, src, records, previous != nil)
		if err != nil {
			return err
		}
		records[name] = src
		return replacePlugin(dir, transaction, name, staged, exists, records)
	})
	if err != nil {
		return nil, "", err
	}
	return manifest, src.Commit, nil
}

func checkDestination(dir, name string, src Source, recs Records, updating bool) (bool, error) {
	identity := sourceIdentity(src.URL)
	for installed, record := range recs {
		if sourceIdentity(record.URL) == identity && installed != name {
			return false, fmt.Errorf("%s was installed as %q and now declares %q", src.URL, installed, name)
		}
	}
	dest := filepath.Join(dir, name)
	info, err := os.Lstat(dest)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("%s is linked for development by ff plugin dev: ff plugin remove %s first", name, name)
	}
	previous, recorded := recs[name]
	movesRelease := updating && previous.TrackRelease && src.TrackRelease && sameRepository(previous.URL, src.URL)
	if (recorded && sourceIdentity(previous.URL) != identity && !movesRelease) || (info != nil && (!recorded || !info.IsDir())) {
		return false, fmt.Errorf("%s already belongs to another installation: ff plugin remove %s first", name, name)
	}
	if info != nil {
		file, err := os.Lstat(filepath.Join(dest, ManifestFile))
		if err != nil {
			return false, err
		}
		if !file.Mode().IsRegular() {
			return false, fmt.Errorf("%s has no regular %s", dest, ManifestFile)
		}
		installed, err := ReadManifest(dest)
		if err != nil {
			return false, err
		}
		if installed.Name != name {
			return false, fmt.Errorf("%s contains plugin %q, expected %q", dest, installed.Name, name)
		}
	}
	return info != nil, nil
}

func sameRepository(old, next string) bool {
	oldRepository, _, _ := strings.Cut(old, "#")
	nextRepository, _, _ := strings.Cut(next, "#")
	return sourceIdentity(oldRepository) == sourceIdentity(nextRepository)
}

func sourceIdentity(source string) string {
	repo, _, _ := strings.Cut(source, "#")
	if strings.HasPrefix(repo, "https://github.com/") {
		return strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git") + source[len(repo):]
	}
	return source
}

func replacePlugin(dir, work, name, staged string, hadOld bool, recs Records) error {
	recordFile := filepath.Join(work, RecordsFile)
	if err := stageRecords(recordFile, recs); err != nil {
		return err
	}
	dest := filepath.Join(dir, name)
	backup := filepath.Join(work, "previous")
	if hadOld {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
	}
	if staged != "" {
		if err := os.Rename(staged, dest); err != nil {
			if hadOld {
				err = errors.Join(err, os.Rename(backup, dest))
			}
			return err
		}
	}
	err := os.Rename(recordFile, filepath.Join(dir, RecordsFile))
	if err == nil {
		return nil
	}
	if staged != "" {
		err = errors.Join(err, os.Rename(dest, staged))
	}
	if hadOld {
		err = errors.Join(err, os.Rename(backup, dest))
	}
	return err
}

func ensurePluginDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Clean(dir))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s must be a plugins directory, not a link or file", dir)
	}
	return nil
}

func changePlugins(dir string, change func(work string, records Records) error) error {
	if err := ensurePluginDir(dir); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".plugins-write.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("locking plugins for a change: %w", err)
	}
	defer lock.Close()
	if err := tryLockPluginFile(lock); err != nil {
		return fmt.Errorf("locking plugins for a change: %w", err)
	}
	defer unlockPluginFile(lock)
	work, err := os.MkdirTemp(dir, ".install-transaction-")
	if err != nil {
		return err
	}
	records, err := readRecords(dir)
	if err == nil {
		err = change(work, records)
	}
	if err != nil {
		backup := filepath.Join(work, "previous")
		if _, statErr := os.Lstat(backup); !errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("%w (original installation preserved in %s)", err, backup)
		}
	}
	return errors.Join(err, os.RemoveAll(work))
}

// Update follows the source selected when a plugin was installed.
func Update(ctx context.Context, dir, name string) (bool, error) {
	recs, err := readRecords(dir)
	if err != nil {
		return false, err
	}
	return updateSource(ctx, dir, name, recs[name], func() ([]IndexEntry, error) { return fetchUpdateIndex(ctx) })
}

func fetchUpdateIndex(ctx context.Context) ([]IndexEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return FetchIndex(ctx, nil)
}

func updateSource(ctx context.Context, dir, name string, src Source, catalog func() ([]IndexEntry, error)) (bool, error) {
	if src.URL == "" {
		return false, fmt.Errorf("%s was not installed from Git", name)
	}
	if src.Pinned() {
		return false, nil
	}
	if src.TrackRelease {
		index, err := catalog()
		if err != nil {
			return false, fmt.Errorf("reading the index: %w", err)
		}
		entry, ok := FindIndex(index, name)
		if !ok {
			return false, IndexNameNotFound(name)
		}
		if err := entry.check(); err != nil {
			return false, err
		}
		release, ok := src.NewerRelease(entry)
		if !ok {
			return false, nil
		}
		_, _, err = installGit(ctx, dir, name, entry.releaseSource(release, true), &src)
		return err == nil, err
	}
	next := src
	next.Commit = ""
	_, commit, err := installGit(ctx, dir, name, next, &src)
	return err == nil && commit != src.Commit, err
}

// URL is the repository the entry installs from, with no subdirectory.
func (entry IndexEntry) URL() string {
	repo, _, _ := strings.Cut(entry.Repository, "#")
	return repo
}

// FindIndex finds an entry by name.
func FindIndex(index []IndexEntry, name string) (IndexEntry, bool) {
	for _, entry := range index {
		if entry.Name == name {
			return entry, true
		}
	}
	return IndexEntry{}, false
}

// Latest is the last commit of an installed plugin's source, without
// installing it, to tell whether an update is waiting.
func Latest(ctx context.Context, src Source) (string, error) {
	repo, _, err := splitRepository(src.URL)
	if err != nil {
		return "", err
	}
	refs := []string{"HEAD"}
	if ref := src.Ref; ref != "" && ref != "HEAD" {
		if strings.HasPrefix(ref, "refs/") {
			refs = []string{ref + "^{}", ref}
		} else {
			tag := "refs/tags/" + ref
			refs = []string{"refs/heads/" + ref, tag + "^{}", tag}
		}
	}
	out, err := git(ctx, "", append([]string{"ls-remote", "--", repo}, refs...)...)
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		for line := range strings.SplitSeq(out, "\n") {
			if commit, name, ok := strings.Cut(line, "\t"); ok && name == ref {
				return commit, nil
			}
		}
	}
	if len(src.Ref) >= 4 && strings.HasPrefix(src.Commit, strings.ToLower(src.Ref)) {
		return src.Commit, nil
	}
	return "", fmt.Errorf("ref %q not found in %s", src.Ref, repo)
}

// Remove deletes an installed plugin, or the link of one in development,
// and its record.
func Remove(dir, name string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	return changePlugins(dir, func(work string, recs Records) error {
		dest := filepath.Join(dir, name)
		if _, err := os.Lstat(dest); err != nil {
			return fmt.Errorf("%s is not installed: %w", name, err)
		}
		if _, ok := recs[name]; !ok {
			return os.RemoveAll(dest)
		}
		delete(recs, name)
		return replacePlugin(dir, work, name, "", true, recs)
	})
}

// LinkDev links a folder in development into the plugins folder, so that
// FiveLeaf loads it from where it is, and again on each change.
func (dirs Dirs) LinkDev(folder string) (*Manifest, error) {
	real, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(real)
	if err != nil {
		return nil, err
	}
	m, err := ReadManifest(abs)
	if err != nil {
		return nil, err
	}
	if err := checkToolboxPage(abs, m); err != nil {
		return nil, err
	}
	if dirs.Bundled != nil {
		if data, err := fs.ReadFile(dirs.Bundled, m.Name+"/"+ManifestFile); err == nil {
			stock, err := ParseManifest(data)
			if err != nil || stock.ToolboxPage() == "" || m.ToolboxPage() == "" {
				return nil, fmt.Errorf("%s is bundled with FiveLeaf", m.Name)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	err = changePlugins(dirs.Plugins, func(work string, _ Records) error {
		dest := filepath.Join(dirs.Plugins, m.Name)
		if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s is installed: ff plugin remove %s first", m.Name, m.Name)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		link := filepath.Join(work, "link")
		if err := os.Symlink(abs, link); err != nil {
			return err
		}
		return os.Rename(link, dest)
	})
	return m, err
}

// SyncInstall installs the plugins of records that the plugins folder
// lacks, at their commits, and removes those it has from records that
// are no longer in them: settings sync from another machine. It returns
// the names it installed and removed.
func SyncInstall(ctx context.Context, dir string, want Records) (installed, removed []string, err error) {
	have, err := readRecords(dir)
	if err != nil {
		return nil, nil, err
	}
	var errs []error
	for name, src := range want {
		if err := ValidName(name); err != nil {
			errs = append(errs, err)
			continue
		}
		var previous *Source
		if record, ok := have[name]; ok {
			previous = &record
		}
		if info, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || previous != nil && *previous == src && info.IsDir() {
				continue
			}
		}
		if _, _, err := installGit(ctx, dir, name, src, previous); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		installed = append(installed, name)
	}
	sort.Strings(installed)
	if len(errs) > 0 {
		return installed, nil, errors.Join(errs...)
	}
	for name := range have {
		if _, ok := want[name]; !ok {
			if err := Remove(dir, name); err != nil {
				errs = append(errs, err)
			} else {
				removed = append(removed, name)
			}
		}
	}
	sort.Strings(removed)
	return installed, removed, errors.Join(errs...)
}

// ScaffoldKind selects the files written by ff plugin new.
type ScaffoldKind int

const (
	ScaffoldPlugin ScaffoldKind = iota
	ScaffoldTheme
	ScaffoldToolbox
	ScaffoldDebugAdapter
	ScaffoldVueTool
	ScaffoldReactTool
)

// Scaffold writes a new plugin into dir: its manifest, an entry module
// with a command, the types of the fiveleaf module, and notes for agents.
// A theme plugin has a theme file instead of an entry module; a Toolbox
// tool has an HTML page and the script behind it.
func Scaffold(dir, name string, theme bool) error {
	kind := ScaffoldPlugin
	if theme {
		kind = ScaffoldTheme
	}
	return ScaffoldKindOf(dir, name, kind)
}

// ScaffoldKindOf writes a new plugin into dir, of kind.
func ScaffoldKindOf(dir, name string, kind ScaffoldKind) error {
	if err := ValidName(name); err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		if entries, _ := os.ReadDir(dir); len(entries) > 0 {
			return fmt.Errorf("%s is not empty", dir)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	title := humanize(strings.ReplaceAll(name, "-", " "))
	// The author is who Git says commits, as the plugin will be committed.
	author, _ := git(context.Background(), dir, "config", "user.name")
	files := map[string]string{}
	switch kind {
	case ScaffoldTheme:
		files[ManifestFile] = fmt.Sprintf(`{
  "name": %q,
  "displayName": %q,
  "version": "0.1.0",
  "description": "",
  "category": "themes",
  "author": %q,
  "contributes": {
    "themes": [{ "id": "%s.theme", "label": %q, "path": "theme.json" }]
  }
}
`, name, title, author, name, title)
		files["theme.json"] = fmt.Sprintf(`{
  "name": %q,
  "base": "dark",
  "colors": {
    "editor": "#1b1b22",
    "chrome": "#101015",
    "panel": "#15151b",
    "text": "#dededf",
    "accent": "#8b7cf6",
    "selection": "#8b7cf64d"
  },
  "syntax": {
    "keyword": "#b99cf7",
    "string": "#a9d48f",
    "comment": "#747481"
  }
}
`, title)
	case ScaffoldDebugAdapter:
		files[ManifestFile] = fmt.Sprintf(`{
  "name": %q,
  "displayName": %q,
  "version": "0.1.0",
  "category": "debug-adapters",
  "author": %q,
  "contributes": {
    "languages": [{ "id": %[1]q, "extensions": [".%[1]s"], "lexer": "python" }],
    "settings": {
      "pythonPath": { "type": "string", "default": "python3", "description": "Python executable with debugpy installed" }
    },
    "debugAdapters": [{
      "id": "%[1]s.debug",
      "name": %[2]q,
      "adapterID": "python",
      "languages": [%[1]q],
      "command": "${settings.pythonPath}",
      "args": ["-m", "debugpy.adapter"],
      "launch": { "program": "${file}", "cwd": "${workspaceFolder}", "console": "internalConsole" }
    }]
  }
}
`, name, title, author)
		files["example."+name] = "message = 'Hello from FiveLeaf'\nprint(message)\n"
		files["README.md"] = fmt.Sprintf(`# %s

This example runs Python code in .%s files using debugpy over standard I/O.

1. Install debugpy in a Python environment with 'python -m pip install debugpy'.
2. Run 'ff plugin dev .' in this folder and open it as a project in FiveLeaf.
3. In **Settings → Plugins → %s**, set **pythonPath** to that environment's Python executable.
4. Open **example.%s**, press F9 on line 2 and press F5. Inspect the value of message in **Run and Debug**.

To support your own language, edit the language ID and file extensions in plugin.json.
Set debugAdapters.languages to the same ID, set command and args to your DAP server,
and supply the adapterID and launch arguments it expects. The server can be written in any language.
`, title, name, title, name)
	case ScaffoldVueTool, ScaffoldReactTool:
		files = scaffoldToolboxFramework(kind, name, title)
		files[ManifestFile] = toolboxTemplateManifest(name, title, author, "dist/index.html")
	case ScaffoldToolbox:
		files[ManifestFile] = toolboxTemplateManifest(name, title, author, "index.html")
		files["index.html"] = fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <link rel="stylesheet" href="style.css">
</head>
<body>
  <main>
    <h1>%s</h1>
    <label for="text">Text</label>
    <input id="text" value="Hello">
    <button id="copy" type="button">Copy</button>
  </main>
  <script type="module" src="app.js"></script>
</body>
</html>
`, title, title)
		files["app.js"] = `const copy = document.querySelector('#copy')
copy.addEventListener('click', () => {
  const text = document.querySelector('#text').value
  window.mygo?.call('Toolbox.Copy', text).catch(console.error)
})

const theme = await window.mygo?.call('Toolbox.Theme').catch(() => null)
if (theme) {
  for (const [role, value] of Object.entries(theme.colors)) {
    document.documentElement.style.setProperty(` + "`--ff-${role}`" + `, value)
  }
  document.documentElement.dataset.theme = theme.dark ? 'dark' : 'light'
}
`
		files["style.css"] = `:root { color-scheme: light dark; font: 14px/1.5 system-ui, sans-serif; }
body { margin: 0; padding: 24px; color: var(--ff-text, #222); background: var(--ff-editor, #fff); }
main { display: grid; gap: 10px; max-width: 560px; }
h1 { margin: 0; font-size: 20px; }
label { color: var(--ff-muted, #666); }
input, button { padding: 8px 10px; border: 1px solid var(--ff-border, #ccc); border-radius: 7px; font: inherit; }
input { color: inherit; background: var(--ff-surface, transparent); }
button { justify-self: start; color: var(--ff-text, #222); background: var(--ff-hover, #f0f0f0); cursor: pointer; }
button:hover { background: var(--ff-active, #e6e6e6); }
`
	default:
		files[ManifestFile] = fmt.Sprintf(`{
  "name": %q,
  "displayName": %q,
  "version": "0.1.0",
  "description": "",
  "category": "tools",
  "author": %q,
  "main": "main.ts",
  "activation": ["onCommand:%s.hello"],
  "contributes": {
    "commands": [{ "id": "%s.hello", "title": "Say Hello" }]
  }
}
`, name, title, author, name, name)
		files["main.ts"] = fmt.Sprintf(`import { commands, editor, window } from "fiveleaf";

commands.register("%s.hello", async () => {
  const ed = await editor.active();
  const name = ed?.document.name ?? "FiveLeaf";
  await window.notify(`+"`Hello from %s, in ${name}`"+`);
});
`, name, title)
		files["fiveleaf.d.ts"] = TypesSource
		files["tsconfig.json"] = `{
  "compilerOptions": { "target": "es2022", "module": "es2022", "strict": true, "noEmit": true },
  "include": ["*.ts", "fiveleaf.d.ts"]
}
`
	}
	files["AGENTS.md"] = AgentsSource
	files[".gitignore"] = "node_modules/\n"
	for f, body := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Watch loads the plugins again a moment after their folders change, and
// a plugin linked for development as its own files change.
func (r *Registry) Watch() (stop func(), err error) {
	return watchDirs(r, 300*time.Millisecond)
}
