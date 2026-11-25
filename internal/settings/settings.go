// Package settings keeps what FiveLeaf remembers between launches, in JSON
// files: the preferences in one, and the state FiveLeaf keeps itself in
// another beside it.
package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fsnotify/fsnotify"
)

// Settings are what FiveLeaf remembers between launches: the preferences,
// in settings.json, and the state FiveLeaf keeps itself, in state.json
// beside it. A window's frame is its StateKey's.
type Settings struct {
	Prefs
	State
	// WrapGen counts the changes of WordWrap.
	WrapGen int

	path      string
	statePath string
	mu        sync.Mutex
	timer     *time.Timer
	// written is what the settings last wrote to their file, which a
	// change of the file that is not theirs differs from, and
	// stateWritten what they last wrote to the state's.
	written      []byte
	stateWritten []byte
	// prefsWritten is called, on a goroutine of its own, each time the
	// settings write a change of the preferences to their file.
	prefsWritten func()
	// seq counts the encodings of the settings, which wmu writes in turn:
	// wrote is the seq of the last written, before which none lands.
	seq   uint64
	wmu   sync.Mutex
	wrote uint64
	// gen counts the changes of the preferences, as Changed and SetPrefs
	// tell of them, prefsSeen is the preferences as gen last counted, and
	// prefsChanged is called with each change Changed tells of.
	gen          uint64
	prefsSeen    []byte
	prefsChanged func()
}

// State is what FiveLeaf keeps itself, not what a person chooses: the
// recent folders, the windows with the folder, the layout and the files
// open in each, and the sessions of the folders closed.
type State struct {
	Recent               []string        `json:"recent"`
	Notifications        []Notification  `json:"notifications,omitempty"`
	NotificationSequence uint64          `json:"notificationSequence,omitempty"`
	RemovedAgentSessions map[string]bool `json:"removedAgentSessions,omitempty"`
	// Windows are the windows open, the one focused last first.
	Windows []*Window `json:"windows,omitempty"`
	// Rail is the order of the windows' IDs on the rail of project tabs,
	// as they were last.
	Rail []int `json:"rail,omitempty"`
	// Sessions are the tabs and the layout of the recent folders no
	// window has open, by folder, which the next window to open one gets
	// back.
	Sessions map[string]*Session `json:"sessions,omitempty"`
}

type Notification struct {
	ID          uint64    `json:"id"`
	Time        time.Time `json:"time"`
	Source      string    `json:"source"`
	Kind        string    `json:"kind,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
}

func (s *Settings) AddNotification(entry Notification) uint64 {
	const limit = 100
	if len(s.Notifications) >= limit {
		s.Notifications = slices.Delete(s.Notifications, 0, len(s.Notifications)-limit+1)
	}
	s.NotificationSequence++
	entry.ID = s.NotificationSequence
	s.Notifications = append(s.Notifications, entry)
	s.Changed()
	return entry.ID
}

func (s *Settings) NotificationIndex(id uint64) int {
	return slices.IndexFunc(s.Notifications, func(entry Notification) bool { return entry.ID == id })
}

func (s *Settings) RemoveNotification(id uint64) bool {
	index := s.NotificationIndex(id)
	if index < 0 {
		return false
	}
	s.Notifications = slices.Delete(s.Notifications, index, index+1)
	s.Changed()
	return true
}

// oldLayout is the layout an older version kept for every window, in the
// state, which the windows and the sessions without one of their own get.
type oldLayout struct {
	SidebarOpen  *bool   `json:"sidebarOpen"`
	SidebarView  string  `json:"sidebarView"`
	SidebarWidth float32 `json:"sidebarWidth"`
	PanelHeight  float32 `json:"panelHeight"`
}

// legacy is what an older version kept in the settings' file besides the
// state: DiffSplit, which DiffLayout "split" is now, and the window as a
// version with one window kept it, Folder, Columns, Column, PanelOpen and
// TerminalOpen, with OpenFiles and ActiveFile its tabs as a version
// without columns kept them. Load makes a window of them.
type legacy struct {
	DiffSplit    bool     `json:"diffSplit"`
	Folder       string   `json:"folder"`
	Columns      []Column `json:"columns"`
	Column       int      `json:"column"`
	PanelOpen    bool     `json:"panelOpen"`
	TerminalOpen bool     `json:"terminalOpen"`
	OpenFiles    []string `json:"openFiles"`
	ActiveFile   string   `json:"activeFile"`
}

// Prefs are the settings a person chooses, which an edit of the file
// applies as FiveLeaf runs: not the windows and the layout it keeps itself.
type Prefs struct {
	// ZoomLevel scales all window content by 1.2 to this power.
	ZoomLevel  float32 `json:"zoomLevel"`
	FontSize   float32 `json:"fontSize"`
	FontFamily string  `json:"fontFamily"`
	// FontWeight is how heavy the code's text is, from 100 to 900.
	FontWeight int `json:"fontWeight"`
	// FontFeatures turn OpenType features of the code's font on and off,
	// as "calt=0" turns off most coding ligatures and "ss01" a stylistic
	// set on.
	FontFeatures []string `json:"fontFeatures,omitempty"`
	// LineHeight is the height of a line of code, a multiple of FontSize.
	LineHeight float32 `json:"lineHeight"`
	// TerminalFontFamily and TerminalFontSize are the terminal's, or
	// those of the code when empty and zero.
	TerminalFontFamily string  `json:"terminalFontFamily,omitempty"`
	TerminalFontSize   float32 `json:"terminalFontSize,omitempty"`
	// TerminalCopyOnSelect copies what is selected in a terminal to the
	// clipboard as the selection ends.
	TerminalCopyOnSelect bool `json:"terminalCopyOnSelect"`
	// AutoSave saves changed files a moment after an edit, and as the
	// keyboard leaves them.
	AutoSave         bool `json:"autoSave"`
	QuickSuggestions bool `json:"quickSuggestions"`
	// WordWrap wraps long lines at the edge of the editor, rather than
	// scrolling sideways to them. A tab may choose otherwise, until this
	// changes: Settings.WrapGen counts its changes.
	WordWrap    bool   `json:"wordWrap,omitempty"`
	LineNumbers string `json:"lineNumbers,omitempty"`
	// TabSize is how many columns a tab takes in the editor, from 1 to 8.
	TabSize int `json:"tabSize"`
	// EditorAgentContext offers selection actions in ordinary files to
	// collect code snippets and send them with review comments.
	EditorAgentContext bool `json:"editorAgentContext,omitempty"`
	// PreviewMermaid draws the blocks of Mermaid of a Markdown preview as
	// the diagrams they make, rather than their source.
	PreviewMermaid bool `json:"previewMermaid"`
	// PreviewMath draws the TeX math of a Markdown preview, between $ and
	// $, $$ and $$, or in a block of math, as the formulas it makes.
	PreviewMath bool `json:"previewMath"`
	// MarkdownMode is how a Markdown file, .mdx included, opens: "" as
	// its source, or "live" as Live Preview, its source styled as it
	// reads.
	MarkdownMode string `json:"markdownMode"`
	// DiffLayout is how diffs show: "split" side by side, "inline" in
	// one column, or "" side by side when they are wide enough to.
	DiffLayout string `json:"diffLayout,omitempty"`
	// DiffCrossFiles is how a diff's change navigation crosses into
	// another file at the first or last change of one: "" says where it
	// is and crosses on one more step that way, "always" crosses at once,
	// and "never" stays in the file.
	DiffCrossFiles string `json:"diffCrossFiles,omitempty"`
	// DiffHighlight is what a diff shows of a line that changed: "line"
	// no more than the line, "char" the characters that changed, or ""
	// the words that changed. What it shows is only so: a diff's block
	// actions are for its hunks of lines, whatever it highlights.
	DiffHighlight string `json:"diffHighlight,omitempty"`
	// DiffWhitespace controls the comparison: "eol" ignores line-end
	// whitespace, "all" ignores whitespace, and "blank" also treats
	// whitespace-only lines as empty. Empty is exact comparison.
	DiffWhitespace string `json:"diffWhitespace,omitempty"`
	// DiffContext is how many unchanged lines a diff shows around a
	// change, the rest folded away.
	DiffContext int `json:"diffContext"`
	// DiffSyncScroll has the sides of a side-by-side diff scroll
	// together. Without it, each side scrolls on its own, and scrolls
	// together again from where the right one was.
	DiffAlignedRows bool `json:"diffAlignedRows"`
	DiffSyncScroll  bool `json:"diffSyncScroll"`
	// MergeAutoApply puts the changes of a conflicted file that no
	// conflict holds in its result when the merge view opens. The
	// conflicts stand, for the hand to decide.
	MergeAutoApply bool `json:"mergeAutoApply"`
	// ScmTree lists the changes in source control in a tree of their
	// folders, rather than by path.
	ScmTree    bool `json:"scmTree,omitempty"`
	SearchTree bool `json:"searchTree,omitempty"`
	// ScmRepositoryOpen remembers whether the repository panel is expanded.
	ScmRepositoryOpen bool `json:"scmRepositoryOpen,omitempty"`
	// ScmRemotes selects Remotes instead of Branches in the repository panel.
	ScmRemotes bool `json:"scmRemotes,omitempty"`
	// Blame shows who last changed the caret's line, and when, after it.
	Blame bool `json:"blame"`
	// Vim edits with vim's keys: the editors start in normal mode.
	Vim bool `json:"vim,omitempty"`
	// EditorMode is "helix" for helix's keys: the editors start in normal
	// mode, with a selection. "vim" is the same as Vim, and "" is neither.
	EditorMode string `json:"editorMode,omitempty"`
	// Keymap selects a built-in or plugin shortcut preset; empty keeps FiveLeaf's keys.
	Keymap string `json:"keymap,omitempty"`
	// VimCtrlKeys has vim's normal mode take ⌃F, ⌃B and ⌃R on Windows and
	// Linux, rather than Find, Toggle Sidebar and Open Recent.
	VimCtrlKeys bool `json:"vimCtrlKeys,omitempty"`
	// Appearance is "light", "dark", or "" to follow the system.
	Appearance string `json:"appearance,omitempty"`
	// WindowOpacity controls the background tint from 0.35 to 1 over the
	// native frosted material; text stays opaque. Without material support,
	// the background stays opaque too.
	WindowOpacity float64 `json:"windowOpacity,omitempty"`
	// MicaEnabled adds a frosted background to the selected color theme.
	MicaEnabled bool `json:"micaEnabled"`
	// MicaAnimation moves the Mica background unless the system asks for
	// reduced motion. On unless turned off.
	MicaAnimation bool `json:"micaAnimation"`
	// MicaFrost sets how much of the theme's surface tint covers the light.
	MicaFrost float64 `json:"micaFrost"`
	// MicaHue is the background light's hue in degrees, from 0 to 360.
	MicaHue float64 `json:"micaHue"`
	// MicaIntensity sets the strength of the background's colored light.
	MicaIntensity float64 `json:"micaIntensity"`
	// MicaSpeed scales the background's motion from 0.25 to 3.
	MicaSpeed float64 `json:"micaSpeed"`
	// SmoothCaret glides the editor's caret to where it goes, as Neovide's
	// does, rather than showing it there at once. On unless turned off.
	SmoothCaret bool `json:"smoothCaret"`
	// Theme is "matrix" for The Matrix, "purplegold" for Purple Gold,
	// "pumpkin" for Jack-o'-Lantern, "witchsbrew" for Witch's Brew or
	// "bloodmoon" for Blood Moon, which are always dark, "claude" for Claude, "qoder" for Qoder,
	// "grokbot" for Grok Bot or "cursor" for Cursor, light or dark as
	// Appearance is, a theme's id from ~/.fiveleaf/themes or a plugin, or
	// "" for FiveLeaf's own.
	Theme string `json:"theme,omitempty"`
	// IconTheme selects file and folder icons independently of the color theme.
	// Empty keeps FiveLeaf's icons; plugin IDs remain saved while unavailable.
	IconTheme string `json:"iconTheme,omitempty"`
	// Themes are themes written in settings.json, by id, which may be a
	// base for another and are chosen with Theme. Each has the format of
	// a theme file in ~/.fiveleaf/themes.
	Themes map[string]json.RawMessage `json:"themes,omitempty"`
	// SidebarRight puts the sidebar right of the editors, rather than left.
	SidebarRight bool `json:"sidebarRight"`
	// ToolboxButton hides the Toolbox button of the sidebar, which opens a
	// tool, while the command palette still lists every tool.
	HideToolboxButton bool `json:"hideToolboxButton,omitempty"`
	// TerminalButtonRight puts the status bar's terminal button at its
	// right, rather than its left.
	TerminalButtonRight bool `json:"terminalButtonRight"`
	// ExplorerCompactFolders shows a chain of single-child folders on one
	// row in the Explorer, with its names separated by slashes.
	ExplorerCompactFolders bool `json:"explorerCompactFolders,omitempty"`
	// PanelPosition is where the terminal panel is: "" or "bottom"
	// below the editors, "left" or "right" at the window's side, past
	// the sidebar.
	PanelPosition string `json:"panelPosition,omitempty"`
	// TerminalLocation is where a new terminal opens: "" in the panel, or
	// "editor" in a tab of the editor area.
	TerminalLocation string `json:"terminalLocation,omitempty"`
	// AgentSessionsClaudeDirs are more config folders of Claude Code whose
	// sessions the Sessions view lists, besides ~/.claude, each
	// ~/.claude-* folder and $CLAUDE_CONFIG_DIR. A folder may start with ~.
	AgentSessionsClaudeDirs []string `json:"agentSessionsClaudeDirs,omitempty"`
	// AgentServers are the ACP and CLI agents available in the Agent view. Only
	// user settings may supply programs; folder settings cannot override them.
	AgentServers map[string]AgentServer `json:"agentServers,omitempty"`
	// AgentRoles are named agent, model and prompt combinations from user settings.
	AgentRoles               map[string]AgentRole         `json:"agentRoles,omitempty"`
	AgentRolePresets         map[string]AgentRoleOverride `json:"agentRolePresets,omitempty"`
	Copilot                  CopilotConfig                `json:"copilot,omitzero"`
	CopilotInlineSuggestions bool                         `json:"copilotInlineSuggestions,omitempty"`
	AISecretFiles            []string                     `json:"aiSecretFiles,omitempty"`
	// ProjectTabs keeps the folders open in one window, a rail on its left
	// choosing which shows, rather than a window for each.
	ProjectTabs bool `json:"projectTabs,omitempty"`
	// ProjectTabStyle is a project's own label, color and icon on the rail
	// of project tabs, by its folder.
	ProjectTabStyle map[string]TabStyle `json:"projectTabStyle,omitempty"`
	// Locale is the language FiveLeaf shows, as "en", "zh-Hans" or "ja", or ""
	// for the one the system's languages prefer.
	Locale string `json:"locale,omitempty"`
	// CrashReports is what FiveLeaf does with the report of a crash at the
	// next launch: "" asks whether to send it to Mon, "always" sends it
	// and "never" doesn't.
	CrashReports string `json:"crashReports,omitempty"`
	// Plugins are the plugins' own settings, by plugin name: each an
	// object of the settings its plugin.json declares, and "enabled":
	// false to turn the plugin off.
	Plugins map[string]json.RawMessage `json:"plugins,omitempty"`
	// FormatOnSave formats a document with a plugin's formatter as it
	// saves, by language, as "go", or "*" for every language.
	FormatOnSave map[string]bool `json:"formatOnSave,omitempty"`
	// Formatters chooses the formatter of a language where plugins offer
	// several: the formatter's id, or its plugin's name.
	Formatters map[string]string `json:"formatters,omitempty"`
}

// AgentServer launches an agent using an executable and separate
// arguments, without a shell. Env adds to the inherited environment.
type AgentServer struct {
	Name             string            `json:"name,omitempty"`
	Command          string            `json:"command"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Protocol         string            `json:"protocol,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Capabilities     []string          `json:"capabilities,omitempty"`
	Disabled         bool              `json:"disabled,omitempty"`
	TimeoutSeconds   int               `json:"timeoutSeconds,omitempty"`
	PromptArgument   bool              `json:"promptArgument,omitempty"`
}

const DefaultAgentTimeoutSeconds = 120

func (s AgentServer) EffectiveProtocol() string {
	if s.Protocol == "" {
		return "acp"
	}
	return s.Protocol
}

func (s AgentServer) ReplyTimeoutSeconds() int {
	if s.TimeoutSeconds == 0 {
		return DefaultAgentTimeoutSeconds
	}
	return s.TimeoutSeconds
}

// Validate checks a launch definition before it is saved or run. Files
// edited outside the settings window are checked again when connecting.
func (s AgentServer) Validate() error {
	if p := s.EffectiveProtocol(); p != "acp" && p != "cli" {
		return fmt.Errorf("protocol must be acp or cli")
	}
	if strings.ContainsAny(s.WorkingDirectory, "\x00\r\n") {
		return fmt.Errorf("working directory is invalid")
	}
	if s.TimeoutSeconds < 0 || s.TimeoutSeconds > 3600 {
		return fmt.Errorf("reply timeout must be between 1 and 3600 seconds")
	}
	if s.PromptArgument && s.EffectiveProtocol() != "cli" {
		return fmt.Errorf("promptArgument is only available for CLI agents")
	}
	seen := make(map[string]bool, len(s.Capabilities))
	for _, tag := range s.Capabilities {
		if tag == "" || strings.ContainsRune(tag, ',') || strings.IndexFunc(tag, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return fmt.Errorf("capability tags must be nonempty and contain no whitespace or commas")
		}
		if seen[tag] {
			return fmt.Errorf("capability tag %q is repeated", tag)
		}
		seen[tag] = true
	}
	if strings.TrimSpace(s.Command) == "" {
		return fmt.Errorf("command is required")
	}
	if s.Command != strings.TrimSpace(s.Command) || strings.ContainsAny(s.Command, "\x00\r\n\t") {
		return fmt.Errorf("command must name one executable")
	}
	if !strings.ContainsAny(s.Command, `/\\`) && strings.Contains(s.Command, " ") {
		return fmt.Errorf("put command arguments in args")
	}
	for _, arg := range s.Args {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("arguments cannot contain NUL")
		}
	}
	for name, value := range s.Env {
		if name == "" || strings.ContainsAny(name, "=\x00\r\n") {
			return fmt.Errorf("environment variable name is invalid")
		}
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("environment variable %q contains NUL", name)
		}
	}
	return nil
}

// TabStyle is a project's own look on the rail: Label, up to two
// characters, in place of its initials or logo, Color, a name of the
// terminal's colors as "red" or "cyan", in place of its own, and Icon, the
// name of a picture in the project-icons folder beside the settings, in
// place of its logo. Each is "" for the project's own.
type TabStyle struct {
	Label string `json:"label,omitempty"`
	Color string `json:"color,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// SetTabStyle keeps st as the look of the project of folder, forgetting
// it when st is the project's own.
func (s *Settings) SetTabStyle(folder string, st TabStyle) {
	if st == (TabStyle{}) {
		delete(s.ProjectTabStyle, folder)
		if len(s.ProjectTabStyle) == 0 {
			s.ProjectTabStyle = nil
		}
	} else {
		if s.ProjectTabStyle == nil {
			s.ProjectTabStyle = map[string]TabStyle{}
		}
		s.ProjectTabStyle[folder] = st
	}
	s.Changed()
}

// Window is a window as the settings keep it: its folder, and its
// session of it.
type Window struct {
	// ID names the window's frame, which the next start gives it back.
	ID       int    `json:"id"`
	Folder   string `json:"folder,omitempty"`
	Detached bool   `json:"detached,omitempty"`
	Session
}

// Session is what a window shows of its folder: the columns of its editor
// area and what their tabs show, Column the one that had the keyboard,
// the panel, the folders open in the explorer, and its layout.
type Session struct {
	Layout
	Columns      []Column   `json:"columns,omitempty"`
	PaneLayout   []PaneTile `json:"paneLayout,omitempty"`
	Column       int        `json:"column,omitempty"`
	FileColumn   int        `json:"fileColumn,omitempty"`
	PanelOpen    bool       `json:"panelOpen,omitempty"`
	TerminalOpen bool       `json:"terminalOpen,omitempty"`
	// Expanded are the explorer's folders showing their files, relative to
	// the window's folder.
	Expanded []string `json:"expanded,omitempty"`
	// Review is the review comments not sent yet.
	Review []ReviewNote `json:"review,omitempty"`
}

// ReviewNote is a review comment on lines of a file, as the settings keep
// it: Commit is the commit whose changes it is on, "" for the work tree,
// and Staged set for the changes staged. OldSide is set for lines removed,
// From and To the lines of the side, To after the last. Snippet is what
// the comment quotes, a diff when Diff is set, and Lines the lines as they
// were, which the comment finds again as the file changes.
type ReviewNote struct {
	// Context keeps a code snippet even when it has no comment.
	Context bool     `json:"context,omitempty"`
	Path    string   `json:"path"`
	Commit  string   `json:"commit,omitempty"`
	Staged  bool     `json:"staged,omitempty"`
	OldSide bool     `json:"oldSide,omitempty"`
	From    int      `json:"from"`
	To      int      `json:"to"`
	Diff    bool     `json:"diff,omitempty"`
	Snippet []string `json:"snippet,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Text    string   `json:"text"`
}

// Layout is how a window lays its folder out: what its sidebar shows,
// whether it is closed and how wide it is, and how tall its panel is at
// the bottom, or how wide it is at a side.
type Layout struct {
	// SidebarView is what the sidebar shows: "" the explorer, "search"
	// the search in the files, "git" the changes for source control.
	SidebarView   string  `json:"sidebarView,omitempty"`
	SidebarClosed bool    `json:"sidebarClosed,omitempty"`
	SidebarWidth  float32 `json:"sidebarWidth,omitempty"`
	PanelHeight   float32 `json:"panelHeight,omitempty"`
	PanelWidth    float32 `json:"panelWidth,omitempty"`
	// CommitFilesOpen shows the panel of the files of a commit's tab, hidden
	// at first, and CommitFilesWidth is how wide it is.
	CommitFilesOpen  bool    `json:"commitFilesOpen,omitempty"`
	CommitFilesWidth float32 `json:"commitFilesWidth,omitempty"`
	// ScmChangesRatio is the share of the multi-repository view for changes.
	// Zero fits the changes to their content, up to half the view.
	ScmChangesRatio float32 `json:"scmChangesRatio,omitempty"`
}

// DefaultLayout is the layout of a first launch.
var DefaultLayout = Layout{SidebarWidth: 248, PanelHeight: 260, PanelWidth: 400, CommitFilesWidth: 240}

// Normalize keeps the layout in its bounds, as base has it where it has
// none, as a session an older version kept.
func (l *Layout) Normalize(base Layout) {
	if l.SidebarWidth == 0 {
		*l = base
	}
	if l.PanelWidth == 0 {
		l.PanelWidth = base.PanelWidth
	}
	if l.CommitFilesWidth == 0 {
		l.CommitFilesWidth = base.CommitFilesWidth
	}
	l.SidebarWidth = clamp(l.SidebarWidth, 160, 600)
	l.PanelWidth = clamp(l.PanelWidth, 160, 2000)
	l.CommitFilesWidth = clamp(l.CommitFilesWidth, 140, 800)
	l.PanelHeight = clamp(l.PanelHeight, 100, 2000)
}

// Column is a column of the editor area, as the settings keep it.
type Column struct {
	Tabs   []Tab   `json:"tabs"`
	Active int     `json:"active"`
	Weight float32 `json:"weight"`
	Locked bool    `json:"locked,omitempty"`
}

// PaneTile groups consecutive columns; a leaf consumes the next saved column.
type PaneTile struct {
	Vertical bool       `json:"vertical,omitempty"`
	Weight   float32    `json:"weight,omitempty"`
	Children []PaneTile `json:"children,omitempty"`
}

// Tab is a tab: the file it shows, or a terminal that starts a new shell on restore.
type Tab struct {
	File     string `json:"file,omitempty"`
	Terminal bool   `json:"terminal,omitempty"`
	// Preview is set for an SVG shown as a picture rather than as text,
	// and Live for a Markdown file shown as Live Preview.
	Preview bool `json:"preview,omitempty"`
	Live    bool `json:"live,omitempty"`
	// Transient is set for the preview tab of its column, which the next
	// file previewed replaces.
	Transient bool `json:"transient,omitempty"`
	// Diff is set for the changes of File rather than the file: "head",
	// "index" or "staged", what they are against. With no File, it is
	// the changes of every file, against that. Layout is the diff's, as
	// the settings' DiffLayout; Split is what an older version kept for
	// "split". "commit:" and a hash is the changes of that commit.
	Diff     string `json:"diff,omitempty"`
	RepoRoot string `json:"repoRoot,omitempty"` // relative to the workspace; empty is its root repository
	Layout   string `json:"layout,omitempty"`
	Split    bool   `json:"split,omitempty"`
	// Full is set for a diff showing every unchanged line.
	Full bool `json:"full,omitempty"`
	// Sel is the selection of a file's editor, its caret at Sel[1], and
	// ScrollX and ScrollY where the tab was scrolled. Wrap is the tab's
	// own choice to wrap its lines, nil to follow WordWrap.
	Sel     [2]int  `json:"sel,omitzero"`
	ScrollX float32 `json:"scrollX,omitempty"`
	ScrollY float32 `json:"scrollY,omitempty"`
	Wrap    *bool   `json:"wrap,omitempty"`
	// Untitled is the file in the untitled folder beside the settings that
	// keeps the text of an untitled document as the app quit, and Number
	// the number of its name.
	Untitled string `json:"untitled,omitempty"`
	Number   int    `json:"number,omitempty"`
}

// MaxDiffContext is as many unchanged lines around a change as a diff
// shows, the rest folded away.
const MaxDiffContext = 100

// Default is the settings of a first launch.
func Default() *Settings {
	return &Settings{
		Prefs: Prefs{
			FontSize: 13, FontFamily: "monospace", FontWeight: 400, LineHeight: 1.55, TabSize: 4, WindowOpacity: 1, SmoothCaret: true, MicaAnimation: true,
			MicaFrost: 0.55, MicaHue: 177, MicaIntensity: 0.8, MicaSpeed: 1,
			AutoSave: true, QuickSuggestions: true, Blame: true, TerminalCopyOnSelect: true, PreviewMermaid: true, PreviewMath: true,
			MarkdownMode:    "live",
			DiffAlignedRows: true, DiffContext: 3, DiffSyncScroll: true, MergeAutoApply: true,
			// The sidebar and its button share the left, while the
			// terminal button sits at the status bar's right.
			SidebarRight:        false,
			TerminalButtonRight: true,
			AgentServers:        map[string]AgentServer{},
			AgentRoles:          map[string]AgentRole{},
			AgentRolePresets:    map[string]AgentRoleOverride{},
		},
	}
}

// Load reads the settings at path, and the state in state.json beside
// it, the defaults where they have none. The state an older version kept
// in the settings' file moves to the state's.
func Load(path string) *Settings {
	s := Default()
	s.path = path
	if path != "" {
		s.statePath = filepath.Join(filepath.Dir(path), "state.json")
	}
	var old legacy
	var layout oldLayout
	data, err := os.ReadFile(path)
	if err == nil {
		s.Prefs.decode(data)
		json.Unmarshal(data, &old)
		// The file as read is no edit of the person's to take again.
		s.written = data
	}
	moved := false
	if state, serr := os.ReadFile(s.statePath); serr == nil {
		json.Unmarshal(state, &s.State)
		json.Unmarshal(state, &layout)
	} else if err == nil {
		json.Unmarshal(data, &s.State)
		json.Unmarshal(data, &layout)
		moved = true
	}
	if old.DiffSplit && s.DiffLayout == "" {
		s.DiffLayout = "split"
	}
	s.normalize()
	// JSON null entries do not describe a window or a saved session.
	s.Windows = slices.DeleteFunc(s.Windows, func(w *Window) bool {
		return w == nil || w.Detached && !slices.ContainsFunc(w.Columns, func(col Column) bool { return len(col.Tabs) > 0 })
	})
	for dir, ses := range s.Sessions {
		if ses == nil {
			delete(s.Sessions, dir)
		}
	}
	if moved {
		s.migrate(old)
	}
	s.layOut(layout)
	if moved {
		s.Flush()
	}
	return s
}

// layOut gives the windows and the sessions an older version kept the
// layout it kept for them all, and keeps each in its bounds.
func (s *Settings) layOut(old oldLayout) {
	base := DefaultLayout
	if old.SidebarWidth != 0 {
		base.SidebarWidth = old.SidebarWidth
	}
	if old.PanelHeight != 0 {
		base.PanelHeight = old.PanelHeight
	}
	base.SidebarView = old.SidebarView
	if old.SidebarOpen != nil {
		base.SidebarClosed = !*old.SidebarOpen
	}
	for _, w := range s.Windows {
		w.Normalize(base)
	}
	for _, ses := range s.Sessions {
		ses.Normalize(base)
	}
}

// Zoom levels keep the content within the UI's supported scale range.
const MinZoomLevel, MaxZoomLevel = -5, 8

// MinWindowOpacity keeps the background tint readable over the desktop.
const MinWindowOpacity = 0.35

// normalize keeps the preferences in their bounds, the defaults where the
// file leaves them out or makes no sense.
func (s *Settings) normalize() { s.Prefs.normalize() }

func (p *Prefs) normalize() {
	if p.WindowOpacity == 0 {
		p.WindowOpacity = 1
	}
	p.WindowOpacity = min(max(p.WindowOpacity, MinWindowOpacity), 1)
	p.MicaFrost = max(0.15, min(p.MicaFrost, 0.95))
	p.MicaHue = max(0, min(p.MicaHue, 360))
	p.MicaIntensity = max(0, min(p.MicaIntensity, 1))
	p.MicaSpeed = max(0.25, min(p.MicaSpeed, 3))
	// Older builds exposed the material as a color theme. Preserve the
	// effect while moving that choice to its own switch.
	if p.Theme == "mica" {
		p.Theme = ""
		p.MicaEnabled = true
	}
	p.ZoomLevel = clamp(p.ZoomLevel, MinZoomLevel, MaxZoomLevel)
	p.FontSize = clamp(p.FontSize, 9, 28)
	if strings.TrimSpace(p.FontFamily) == "" {
		p.FontFamily = "monospace"
	}
	if p.FontWeight == 0 {
		p.FontWeight = 400
	}
	p.FontWeight = min(max(p.FontWeight, 100), 900)
	if p.LineHeight == 0 {
		p.LineHeight = 1.55
	}
	p.LineHeight = clamp(p.LineHeight, 1, 3)
	if p.LineNumbers != "relative" {
		p.LineNumbers = ""
	}
	if p.TabSize == 0 {
		p.TabSize = 4
	}
	p.TabSize = min(max(p.TabSize, 1), 8)
	p.FontFeatures = slices.DeleteFunc(p.FontFeatures, func(f string) bool { return strings.TrimSpace(f) == "" })
	if p.TerminalFontSize != 0 {
		p.TerminalFontSize = clamp(p.TerminalFontSize, 9, 28)
	}
	switch p.DiffHighlight {
	case "line", "char":
	default:
		p.DiffHighlight = ""
	}
	switch p.DiffWhitespace {
	case "eol", "all", "blank":
	default:
		p.DiffWhitespace = ""
	}
	p.DiffContext = min(max(p.DiffContext, 0), MaxDiffContext)

	// "" and "bottom" are the same panel position.
	if p.PanelPosition != "left" && p.PanelPosition != "right" {
		p.PanelPosition = ""
	}
	if p.TerminalLocation != "editor" {
		p.TerminalLocation = ""
	}
	switch p.MarkdownMode {
	case "", "source":
		p.MarkdownMode = ""
	default:
		p.MarkdownMode = "live"
	}
	// The themes were appearances once.
	if p.Appearance == "matrix" || p.Appearance == "claude" {
		p.Theme, p.Appearance = p.Appearance, ""
	}
}

// migrate makes a window of the one an older version kept.
func (s *Settings) migrate(old legacy) {
	if len(s.Windows) > 0 || old.Folder == "" && len(old.Columns) == 0 && len(old.OpenFiles) == 0 {
		return
	}
	w := &Window{Folder: old.Folder, Session: Session{
		Columns: old.Columns, Column: old.Column, PanelOpen: old.PanelOpen, TerminalOpen: old.TerminalOpen,
	}}
	if len(w.Columns) == 0 && len(old.OpenFiles) > 0 {
		// The files of one column.
		cs := Column{Active: -1, Weight: 1}
		for _, p := range old.OpenFiles {
			if p == old.ActiveFile {
				cs.Active = len(cs.Tabs)
			}
			cs.Tabs = append(cs.Tabs, Tab{File: p})
		}
		w.Columns, w.Column = []Column{cs}, 0
	}
	s.Windows = []*Window{w}
}

// Path is the file the settings are kept in, "" for none.
func (s *Settings) Path() string { return s.path }

// NewWindow keeps a new window, focused first, and names its frame by
// the first ID no other window has. It is laid out as the window focused
// last is.
func (s *Settings) NewWindow() *Window {
	id := 0
	for slices.ContainsFunc(s.Windows, func(w *Window) bool { return w.ID == id }) {
		id++
	}
	w := &Window{ID: id}
	w.Layout = DefaultLayout
	if len(s.Windows) > 0 {
		w.Layout = s.Windows[0].Layout
	}
	s.Windows = slices.Insert(s.Windows, 0, w)
	s.Changed()
	return w
}

// InRail returns the windows in the order of the rail of project tabs:
// those it has in its order, then the others as the windows are.
func (s *Settings) InRail() []*Window {
	ws := slices.Clone(s.Windows)
	at := func(w *Window) int {
		if i := slices.Index(s.Rail, w.ID); i >= 0 {
			return i
		}
		return len(s.Rail)
	}
	slices.SortStableFunc(ws, func(a, b *Window) int { return at(a) - at(b) })
	return ws
}

// SetRail keeps the order of the windows on the rail of project tabs.
func (s *Settings) SetRail(ws []*Window) {
	ids := make([]int, len(ws))
	for i, w := range ws {
		ids[i] = w.ID
	}
	if !slices.Equal(ids, s.Rail) {
		s.Rail = ids
		s.Changed()
	}
}

// Focused puts w first, as the window Focused last.
func (s *Settings) Focused(w *Window) {
	i := slices.Index(s.Windows, w)
	if i <= 0 {
		return
	}
	s.Windows = slices.Insert(slices.Delete(s.Windows, i, i+1), 0, w)
	s.Changed()
}

// Forget lets go of a window that closed, keeping its session for the
// next window to open its folder.
func (s *Settings) Forget(w *Window) {
	s.Stash(w)
	s.Windows = slices.DeleteFunc(s.Windows, func(o *Window) bool { return o == w })
	s.Changed()
}

// Stash keeps the session of w's folder, which the folder's next window
// gets back, while it is recent or has a session kept after its recent
// entry was removed. A session with nothing open lets the folder's go.
func (s *Settings) Stash(w *Window) {
	if w.Folder == "" || w.Detached {
		return
	}
	dir := filepath.Clean(w.Folder)
	if !slices.Contains(s.Recent, dir) && s.Sessions[dir] == nil {
		return
	}
	empty := len(w.Expanded) == 0 && len(w.Review) == 0
	for _, c := range w.Columns {
		empty = empty && len(c.Tabs) == 0
	}
	if empty {
		delete(s.Sessions, dir)
	} else {
		if s.Sessions == nil {
			s.Sessions = map[string]*Session{}
		}
		ses := w.Session
		s.Sessions[dir] = &ses
	}
	s.Changed()
}

// TakeSession returns the session kept of dir, which is no longer kept
// once a window has it, or nil for none.
func (s *Settings) TakeSession(dir string) *Session {
	dir = filepath.Clean(dir)
	ses := s.Sessions[dir]
	if ses != nil {
		delete(s.Sessions, dir)
		s.Changed()
	}
	return ses
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }

// Changed saves the settings a moment after the last change.
func (s *Settings) Changed() {
	prefs, state := s.Encode()
	s.mu.Lock()
	var fn func()
	if !bytes.Equal(prefs, s.prefsSeen) {
		s.gen++
		s.prefsSeen = prefs
		fn = s.prefsChanged
	}
	if s.path != "" {
		if s.timer != nil {
			s.timer.Stop()
		}
		s.seq++
		seq := s.seq
		s.timer = time.AfterFunc(400*time.Millisecond, func() { s.write(seq, prefs, state) })
	}
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Gen counts the changes of the preferences: what is made of them, as
// the preferences of a folder over them, is made again as it changes.
func (s *Settings) Gen() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// OnPrefsChanged calls fn each time Changed finds the preferences
// changed, as a menu changes one, on the goroutine of Changed.
func (s *Settings) OnPrefsChanged(fn func()) {
	s.mu.Lock()
	s.prefsChanged = fn
	s.mu.Unlock()
}

// Flush saves the settings now, as the app quits.
func (s *Settings) Flush() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	prefs, state := s.Encode()
	s.seq++
	seq := s.seq
	s.mu.Unlock()
	s.write(seq, prefs, state)
}

// Encode is the preferences as the settings' file keeps them, and the
// state as the state's does.
func (s *Settings) Encode() (prefs, state []byte) {
	prefs, _ = json.MarshalIndent(s.Prefs, "", "  ")
	state, _ = json.MarshalIndent(s.State, "", "  ")
	return prefs, state
}

// write writes the files that changed since the settings last wrote
// them, as of seq: most changes are of the state only. One of an older
// encoding than the last written, as a timer that fired as Flush ran,
// writes nothing.
func (s *Settings) write(seq uint64, prefs, state []byte) {
	s.wmu.Lock()
	if seq < s.wrote {
		s.wmu.Unlock()
		return
	}
	s.wrote = seq
	s.mu.Lock()
	samePrefs, sameState := bytes.Equal(prefs, s.written), bytes.Equal(state, s.stateWritten)
	fn := s.prefsWritten
	s.mu.Unlock()
	// wmu keeps a watcher from reading between the write and cache update.
	// Leave mu free during disk I/O for the preferences read by each frame.
	// Failed writes remain different from the cache and can be retried.
	wrotePrefs := false
	if !samePrefs {
		wrotePrefs = WriteUserPrefs(s.path, prefs) == nil
	}
	wroteState := false
	if !sameState && s.statePath != "" {
		wroteState = WriteFile(s.statePath, state) == nil
	}
	s.mu.Lock()
	if wrotePrefs {
		s.written = prefs
	}
	if wroteState {
		s.stateWritten = state
	}
	s.mu.Unlock()
	s.wmu.Unlock()
	// Outside wmu, as fn may wait for the main thread, which may wait to
	// write.
	if wrotePrefs && fn != nil {
		fn()
	}
}

// WriteFile writes data to path through a file of its own beside it, so
// that a watcher never reads half of it, nor two writers' halves.
func WriteFile(path string, data []byte) error {
	return writeFile(path, data, 0o644)
}

// WriteUserPrefs replaces a user's settings with a private file.
func WriteUserPrefs(path string, data []byte) error {
	return writeFile(path, data, 0o600)
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0o755)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && mode != 0o600 {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// AddRecent puts dir first of the recent folders, of which it keeps ten,
// and lets go of sessions evicted by that limit. Sessions whose recent
// entries were explicitly removed remain available.
func (s *Settings) AddRecent(dir string) {
	s.Recent = slices.DeleteFunc(s.Recent, func(d string) bool { return d == dir })
	s.Recent = slices.Insert(s.Recent, 0, dir)
	if len(s.Recent) > 10 {
		for _, d := range s.Recent[10:] {
			delete(s.Sessions, d)
		}
		s.Recent = s.Recent[:10]
	}
}

// RecentExcept returns the recent folders but dir, and those that are not
// on the disk now, which stay in the list for when they come back, as a
// drive plugged in again.
func (s *Settings) RecentExcept(dir string) []string {
	var out []string
	for _, d := range s.Recent {
		if d == dir {
			continue
		}
		// A remote folder is looked at as it opens.
		if strings.HasPrefix(d, "/ssh:") {
			out = append(out, d)
		} else if info, err := os.Stat(d); err == nil && info.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// RemoveRecent removes only the recent entry, keeping the folder's saved
// session and leaving its files and open windows alone.
func (s *Settings) RemoveRecent(dir string) {
	old := len(s.Recent)
	s.Recent = slices.DeleteFunc(s.Recent, func(d string) bool { return d == dir })
	if len(s.Recent) != old {
		// An open project's live session belongs to its window. Keep a
		// snapshot, even when empty, so Stash can refresh it on close
		// after this entry no longer appears in the recent list.
		if s.Sessions[dir] == nil {
			for _, w := range s.Windows {
				if !w.Detached && filepath.Clean(w.Folder) == dir {
					if s.Sessions == nil {
						s.Sessions = map[string]*Session{}
					}
					ses := w.Session
					s.Sessions[dir] = &ses
					break
				}
			}
		}
		s.Changed()
	}
}

// ClearRecent forgets the recent folders, and the sessions kept of them.
func (s *Settings) ClearRecent() {
	s.Recent, s.Sessions = nil, nil
	s.Changed()
}

// SetPrefs makes p the preferences, as a change of the file has them,
// and returns those it replaces. A change of WordWrap counts in WrapGen.
func (s *Settings) SetPrefs(p Prefs) (old Prefs) {
	old = s.Prefs
	s.Prefs = p
	if p.WordWrap != old.WordWrap {
		s.WrapGen++
	}
	s.mu.Lock()
	s.gen++
	s.mu.Unlock()
	return old
}

// Watch calls changed, on a goroutine of its own, with the preferences
// of the file each time something other than the settings writes it, as
// a person saving it in an editor. A file that does not parse is let be
// until it does. stop stops watching.
func (s *Settings) Watch(changed func(Prefs)) (stop func(), err error) {
	if s.path == "" {
		return func() {}, nil
	}
	dir := filepath.Dir(s.path)
	os.MkdirAll(dir, 0o755)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	// The folder rather than the file, which saves replace: the settings'
	// own, and most editors'.
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}
	go func() {
		var settle <-chan time.Time
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Clean(ev.Name) == filepath.Clean(s.path) && ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
					// Once a save that writes the file in pieces is done.
					settle = time.After(80 * time.Millisecond)
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			case <-settle:
				settle = nil
				if p, ok := s.edited(); ok {
					changed(p)
				}
			}
		}
	}()
	return func() { w.Close() }, nil
}

// edited reads the preferences of the file, unless the settings wrote it
// as it is or it does not parse.
func (s *Settings) edited() (Prefs, bool) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, p, ok := s.read()
	return p, ok
}

// Edited returns the preferences of the file as a person saved it, which
// Watch may not have told of yet, and takes the file as read: Watch then
// tells of it only as it changes again. ok is false for a file the
// settings wrote as it is, or that does not parse.
func (s *Settings) Edited() (p Prefs, ok bool) {
	if s.path == "" {
		return Prefs{}, false
	}
	// Not as a write lands, whose file this would take for the person's.
	s.wmu.Lock()
	defer s.wmu.Unlock()
	data, p, ok := s.read()
	if ok {
		s.mu.Lock()
		s.written = data
		s.mu.Unlock()
	}
	return p, ok
}

func (s *Settings) read() ([]byte, Prefs, bool) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, Prefs{}, false
	}
	s.mu.Lock()
	ours := bytes.Equal(data, s.written)
	s.mu.Unlock()
	if ours {
		return nil, Prefs{}, false
	}
	f := Default()
	if f.Prefs.decode(data) != nil {
		return nil, Prefs{}, false
	}
	f.normalize()
	return data, f.Prefs, true
}

func (p *Prefs) decode(data []byte) error {
	file := struct {
		*Prefs
		Mode    *string `json:"markdownMode"`
		Preview *bool   `json:"markdownPreview"`
	}{Prefs: p}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	if file.Mode != nil {
		p.MarkdownMode = *file.Mode
	} else if file.Preview != nil && !*file.Preview {
		p.MarkdownMode = ""
	}
	return nil
}

// OnPrefsWritten calls fn, on a goroutine of its own, each time the
// settings write a change of the preferences to their file, as a menu
// changing one, though not as the person saves the file: Watch tells of
// that.
func (s *Settings) OnPrefsWritten(fn func()) {
	s.mu.Lock()
	s.prefsWritten = fn
	s.mu.Unlock()
}

// PrefKeys are the keys of the preferences in the settings' file.
var PrefKeys = func() []string {
	var keys []string
	t := reflect.TypeFor[Prefs]()
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}()
