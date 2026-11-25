// Package plugin runs FiveLeaf's plugins: folders with a plugin.json and an
// entry module of JavaScript or TypeScript, each in a moejs runtime of its
// own, on a goroutine of its own. The app sees them through a Registry,
// and they see the app through the Host it implements.
package plugin

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"fiveleaf/internal/keymap"
	"fiveleaf/internal/settings"
)

// ManifestFile is the name of a plugin's manifest in its folder.
const ManifestFile = "plugin.json"

const mayoiCompatibilityVersion = "0.20.1"

// Manifest is what plugin.json says of a plugin.
type Manifest struct {
	Name         string            `json:"name"`
	DisplayName  string            `json:"displayName,omitempty"`
	DisplayNames map[string]string `json:"displayNames,omitempty"`
	Version      string            `json:"version,omitempty"`
	Description  string            `json:"description,omitempty"`
	// Category groups the plugin in the marketplace and the Plugins page.
	// A plugin has one category; tags are only search words.
	Category string `json:"category,omitempty"`
	// Author is who wrote the plugin, as the Plugins page shows it.
	Author string `json:"author,omitempty"`
	// FiveLeaf is the versions of FiveLeaf the plugin works with, as ">=0.1.0".
	FiveLeaf string `json:"fiveleaf,omitempty"`
	// Mayoi is the version requirement used by existing plugins.
	Mayoi string `json:"mayoi,omitempty"`
	// Main is the entry module, relative to the folder: .ts, .js or .mjs.
	Main string `json:"main,omitempty"`
	// Activation are the events that start the plugin: onCommand:<id>,
	// onLanguage:<lang>, onView:<id>, onPreview:<id>, workspaceContains:<glob>,
	// onStartup, or * for any.
	Activation []string `json:"activation,omitempty"`
	// Enabled is whether the plugin starts on, until its settings say;
	// nil is on.
	Enabled     *bool       `json:"enabled,omitempty"`
	Contributes Contributes `json:"contributes"`
	// LegacyToolbox is the old string form of contributes.toolbox, kept so
	// a plugin.json written before tools had pages still reads.
	LegacyToolbox string `json:"-"`
}

// Contributes are what a plugin adds to FiveLeaf, known without running it.
type Contributes struct {
	Commands        []CommandContrib          `json:"commands,omitempty"`
	Keybindings     []KeyContrib              `json:"keybindings,omitempty"`
	Keymaps         []KeymapContrib           `json:"keymaps,omitempty"`
	Menus           map[string][]MenuContrib  `json:"menus,omitempty"`
	Settings        map[string]SettingContrib `json:"settings,omitempty"`
	StatusItems     []StatusContrib           `json:"statusItems,omitempty"`
	Views           []ViewContrib             `json:"views,omitempty"`
	Previews        []PreviewContrib          `json:"previews,omitempty"`
	Themes          []ThemeContrib            `json:"themes,omitempty"`
	IconThemes      []IconThemeContrib        `json:"iconThemes,omitempty"`
	Languages       []LanguageContrib         `json:"languages,omitempty"`
	Snippets        []SnippetContrib          `json:"snippets,omitempty"`
	Formatters      []FormatterContrib        `json:"formatters,omitempty"`
	Toolbox         *ToolboxContrib           `json:"toolbox,omitempty"`
	TerminalLinks   []LinkContrib             `json:"terminalLinks,omitempty"`
	Webviews        []WebviewContrib          `json:"webviews,omitempty"`
	LanguageServers []ServerContrib           `json:"languageServers,omitempty"`
	DebugAdapters   []DebugAdapterContrib     `json:"debugAdapters,omitempty"`
	Agents          []AgentContrib            `json:"agents,omitempty"`
	AgentServers    []AgentServerContrib      `json:"agentServers,omitempty"`
}

// CommandContrib is a command: its id, which starts with the plugin's
// name, and its title, with titles in other languages as "title:zh-Hans".
type CommandContrib struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Titles map[string]string `json:"-"`
	// Arg names what the command takes from `ff run id=arg`, as
	// "<text>"; "" takes none.
	Arg string `json:"arg,omitempty"`
}

func (c *CommandContrib) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	type plain CommandContrib
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*c = CommandContrib(p)
	for k, v := range raw {
		if tag, ok := strings.CutPrefix(k, "title:"); ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				if c.Titles == nil {
					c.Titles = map[string]string{}
				}
				c.Titles[tag] = s
			}
		}
	}
	return nil
}

// TitleIn is the command's title in the language of tag, or its own.
func (c CommandContrib) TitleIn(tag string) string {
	return localizedTitle(c.Title, c.Titles, tag)
}

func localizedTitle(fallback string, titles map[string]string, tag string) string {
	if title := titles[tag]; title != "" {
		return title
	}
	if base, _, ok := strings.Cut(tag, "-"); ok {
		if title := titles[base]; title != "" {
			return title
		}
	}
	return fallback
}

// KeyContrib binds keys, in keybindings.json's format, to a command; Mac
// is the keys on macOS, when they differ.
type KeyContrib struct {
	Key     string `json:"key"`
	Mac     string `json:"mac,omitempty"`
	Command string `json:"command"`
	When    string `json:"when,omitempty"`
}

// KeymapContrib is a shortcut preset selected in the settings window.
// Unlike a command's suggested shortcut, it can replace built-in keys.
type KeymapContrib struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Bindings []keymap.Entry `json:"bindings"`
}

// MenuContrib puts a command in a menu: "plugins" (the menu bar's),
// "editor/context", "explorer/context" or "tab/context".
type MenuContrib struct {
	Command string `json:"command"`
	When    string `json:"when,omitempty"`
}

// SettingContrib is a setting of the plugin, kept under
// plugins.<name>.<key> in settings.json.
type SettingContrib struct {
	Type        string          `json:"type"`
	Default     json.RawMessage `json:"default,omitempty"`
	Description string          `json:"description,omitempty"`
	Enum        []string        `json:"enum,omitempty"`
	Minimum     *int64          `json:"minimum,omitempty"`
	Maximum     *int64          `json:"maximum,omitempty"`
}

func (setting SettingContrib) ArgumentValue(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("expected %s", setting.Type)
	}
	switch setting.Type {
	case "string":
		var value string
		if json.Unmarshal(raw, &value) == nil {
			if len(setting.Enum) > 0 && !slices.Contains(setting.Enum, value) {
				return "", fmt.Errorf("expected one of %q", setting.Enum)
			}
			return value, nil
		}
	case "boolean":
		var value bool
		if json.Unmarshal(raw, &value) == nil {
			return strconv.FormatBool(value), nil
		}
	case "integer":
		var value int64
		if json.Unmarshal(raw, &value) == nil {
			if setting.Minimum != nil && value < *setting.Minimum {
				return "", fmt.Errorf("minimum is %d", *setting.Minimum)
			}
			if setting.Maximum != nil && value > *setting.Maximum {
				return "", fmt.Errorf("maximum is %d", *setting.Maximum)
			}
			return strconv.FormatInt(value, 10), nil
		}
	case "number":
		var value float64
		if json.Unmarshal(raw, &value) == nil {
			return string(raw), nil
		}
	}
	return "", fmt.Errorf("expected %s", setting.Type)
}

// StatusContrib is an item of the status bar, which the plugin fills.
type StatusContrib struct {
	ID    string `json:"id"`
	Align string `json:"align,omitempty"` // "left" or "right", the default
	// Priority orders the items of a side, the highest first.
	Priority int `json:"priority,omitempty"`
}

// ViewContrib is a view the plugin renders: in the sidebar beside the
// Explorer, or in a tab.
type ViewContrib struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Icon     string `json:"icon,omitempty"`
	Location string `json:"location,omitempty"` // "sidebar", the default, or "tab"
}

// PreviewContrib previews the files a glob matches beside their source,
// as the plugin renders them: "svg", "markdown" or "view".
type PreviewContrib struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	Pattern string `json:"pattern"`
	Output  string `json:"output"`
}

// ThemeContrib is a theme: a palette in JSON, or a VS Code theme, in the
// plugin's folder.
type ThemeContrib struct {
	ID           string            `json:"id"`
	Label        string            `json:"label"`
	Labels       map[string]string `json:"labels,omitempty"`
	Descriptions map[string]string `json:"descriptions,omitempty"`
	Path         string            `json:"path"`
	DarkPath     string            `json:"darkPath,omitempty"`
	// Dark tells a theme for dark windows; a theme file may say too.
	Dark bool `json:"dark,omitempty"`
}

// IconThemeContrib names a file and folder icon pack in the plugin's folder.
type IconThemeContrib struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Path  string `json:"path"`
}

// LanguageContrib tells FiveLeaf of a language: its files, how its comments
// start, and which Chroma lexer highlights it.
type LanguageContrib struct {
	ID         string   `json:"id"`
	Extensions []string `json:"extensions,omitempty"`
	Filenames  []string `json:"filenames,omitempty"`
	Lexer      string   `json:"lexer,omitempty"`
	Comment    string   `json:"lineComment,omitempty"`
	Indent     int      `json:"indent,omitempty"`
}

// SnippetContrib is a file of snippets in VS Code's format for a language,
// or every language for "*".
type SnippetContrib struct {
	Language string `json:"language"`
	Path     string `json:"path"`
}

// FormatterContrib formats the documents of languages.
type FormatterContrib struct {
	ID          string            `json:"id"`
	Title       string            `json:"title,omitempty"`
	Languages   []string          `json:"languages"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	SettingArgs map[string]string `json:"settingArgs,omitempty"`
}

// LinkContrib turns what a pattern matches in a terminal into a link that
// runs a command with the match.
type LinkContrib struct {
	Pattern string `json:"pattern"`
	Command string `json:"command"`
}

// WebviewContrib is a panel of HTML from the plugin's folder.
type WebviewContrib struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
}

// ServerContrib is a language server, started for its languages.
type ServerContrib struct {
	ID          string            `json:"id"`
	Languages   []string          `json:"languages"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	SettingArgs map[string]string `json:"settingArgs,omitempty"`
	Options     json.RawMessage   `json:"initializationOptions,omitempty"`
	// Resolve prepares a launch description, using workspace/settings JSON on
	// stdin and returning command, args and LSP options as JSON on stdout.
	Resolve *ServerResolver `json:"resolve,omitempty"`
}

type ServerResolver struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// ToolboxContrib adds a tool to Toolbox. Page is an HTML file in the
// plugin's folder; Features are the searchable names of what it does.
type ToolboxContrib struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Page        string   `json:"page,omitempty"`
	Builtin     string   `json:"builtin,omitempty"`
	Features    []string `json:"features,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
}

// ToolboxPage is the page a plugin adds to Toolbox, and the name the
// palette and the window route by. A tool that still declares its page as
// the old string, "base64" or "time", reads as the tool of that page.
func (m *Manifest) ToolboxPage() string {
	if m.Contributes.Toolbox != nil {
		if m.Contributes.Toolbox.ID != "" {
			return m.Contributes.Toolbox.ID
		}
		return path.Base(m.Contributes.Toolbox.Page)
	}
	return m.LegacyToolbox
}

// DebugAdapterContrib is a Debug Adapter Protocol server, started for its
// languages. Launch, or Resolve, says how the adapter starts a session.
type DebugAdapterContrib struct {
	ID        string            `json:"id"`
	Name      string            `json:"name,omitempty"`
	AdapterID string            `json:"adapterID,omitempty"`
	Languages []string          `json:"languages"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Options   json.RawMessage   `json:"initializationOptions,omitempty"`
	// Transport says how the adapter listens: "" or "stdio" reads its
	// standard input and output, "tcp" connects to the port it prints.
	Transport *DebugTransport `json:"transport,omitempty"`
	// Launch and Attach are the default arguments of the requests, with
	// ${file}, ${workspaceFolder}, ${fileDirname} and ${fileBasename}
	// replaced.
	Launch json.RawMessage `json:"launch,omitempty"`
	Attach json.RawMessage `json:"attach,omitempty"`
	// Resolve runs a helper, before the adapter starts, that returns the
	// command, args, transport and launch arguments for this workspace.
	// Its stdin is JSON {root, file, settings}; its stdout is one JSON
	// object with overrides for the declared adapter and launch arguments.
	Resolve *DebugResolver `json:"resolve,omitempty"`
}

// DebugResolver is a helper that prepares a debug launch: for adapters
// whose command or launch arguments depend on the workspace, as java-debug
// does, or that find a port first.
type DebugResolver struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// DebugTransport is how to reach an adapter that does not speak over its
// standard input and output.
type DebugTransport struct {
	// Kind is "" or "stdio", or "tcp".
	Kind string `json:"kind,omitempty"`
	// Address is the fixed host and port a TCP adapter listens on.
	Address string `json:"address,omitempty"`
	// Ready is the text the adapter prints once it listens.
	Ready string `json:"ready,omitempty"`
}

func (transport *DebugTransport) Direct() bool {
	return transport != nil && transport.Kind == "tcp" && transport.Address != ""
}

func (transport *DebugTransport) Validate() error {
	if transport == nil {
		return nil
	}
	switch transport.Kind {
	case "", "stdio":
		if transport.Address != "" || transport.Ready != "" {
			return errors.New("stdio transport cannot use address or ready")
		}
	case "tcp":
		if (transport.Address == "") == (transport.Ready == "") {
			return errors.New("tcp transport requires either a fixed address or a ready message")
		}
		if transport.Ready != "" {
			if strings.TrimSpace(transport.Ready) == "" || strings.ContainsAny(transport.Ready, "\r\n\x00") {
				return errors.New("tcp ready message must be text on one line")
			}
			return nil
		}
		host, port, err := net.SplitHostPort(transport.Address)
		number, portErr := strconv.ParseUint(port, 10, 16)
		if err != nil || host == "" || strings.ContainsAny(host, " \t\r\n\x00") || portErr != nil || number == 0 {
			return fmt.Errorf("tcp address %q needs a host and port from 1 to 65535", transport.Address)
		}
	default:
		return fmt.Errorf("unknown transport %q", transport.Kind)
	}
	return nil
}

// AgentServerContrib adds an ACP or CLI agent to the Agent view. It needs no
// JavaScript entry point. A command with a relative path is resolved in
// the plugin's folder; a bare executable name is found on PATH.
type AgentServerContrib struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Command          string            `json:"command"`
	LocalCommand     string            `json:"localCommand,omitempty"`
	LocalCommandEnv  string            `json:"localCommandEnv,omitempty"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Protocol         string            `json:"protocol,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Capabilities     []string          `json:"capabilities,omitempty"`
	Disabled         bool              `json:"disabled,omitempty"`
	TimeoutSeconds   int               `json:"timeoutSeconds,omitempty"`
	PromptArgument   bool              `json:"promptArgument,omitempty"`
}

// AgentContrib is a coding agent whose sessions the Sessions view lists:
// of the files Sessions matches, or else of what the plugin's function
// registered with agents.register(id, fn) returns. Resume is the shell
// command that resumes one, {id}, {folder} and {title} the session's.
type AgentContrib struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// App is the name it gives itself in what it says it does (OSC 7501),
	// the id by default.
	App string `json:"app,omitempty"`
	// Icon is the shapes of an SVG of a 24 by 24 view, drawn in strokes, a
	// whole SVG, or the name of one of FiveLeaf's icons.
	Icon string `json:"icon,omitempty"`
	// Tint is the color of its icon: one hex color, or one for a dark
	// window and one for a light one.
	Tint     Tint           `json:"tint,omitempty"`
	Resume   string         `json:"resume"`
	Sessions *AgentSessions `json:"sessions,omitempty"`
}

// AgentSessions are the files of an agent's sessions, a session each:
// those Glob matches, of JSON or of a JSON object a line, and where in
// them each field is, a path of keys by dots.
type AgentSessions struct {
	Glob    string `json:"glob"`
	Format  string `json:"format,omitempty"` // "json", the default, or "jsonl"
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Folder  string `json:"folder,omitempty"`
	Updated string `json:"updated,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
}

// Tint is a color in a dark window and in a light one, which JSON gives
// as one hex color for both or two.
type Tint [2]string

func (t *Tint) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*t = Tint{one, one}
		return nil
	}
	var two []string
	if err := json.Unmarshal(data, &two); err != nil || len(two) != 2 {
		return errors.New(`tint must be a hex color or two, as "#d97757" or ["#d97757", "#c15f3c"]`)
	}
	*t = Tint{two[0], two[1]}
	return nil
}

var hexRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// toolIDRe is the id of a Toolbox tool, which the toolbox window routes
// a page by: unlike a command, it need not start with the plugin's name,
// so several plugins can replace a tool with one of the same id.
var toolIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// Reserved are the prefixes of FiveLeaf's own command ids, which no plugin
// may take as its name.
var Reserved = []string{
	"file", "edit", "sel", "go", "view", "git", "review", "remote", "account",
	"editor", "image", "mermaid", "plugins", "script", "fiveleaf", "mayoi",
}

// ValidName reports whether name may name a plugin.
func ValidName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("plugin name %q must be lowercase letters, digits and hyphens, starting with a letter", name)
	}
	for _, r := range Reserved {
		if name == r {
			return fmt.Errorf("plugin name %q is reserved", name)
		}
	}
	return nil
}

// ParseManifest reads a manifest and checks it.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(keymap.StripJSONC(data), &m); err != nil {
		if !missingToolboxString(err) {
			return nil, fmt.Errorf("%s: %v", ManifestFile, err)
		}
		// The old string form of contributes.toolbox is read below,
		// without the object the field expects. The failed pass left a
		// zero ToolboxContrib, which would fail the check.
		m = Manifest{}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(keymap.StripJSONC(data), &raw); err != nil {
			return nil, fmt.Errorf("%s: %v", ManifestFile, err)
		}
		var contributes map[string]json.RawMessage
		json.Unmarshal(raw["contributes"], &contributes)
		delete(contributes, "toolbox")
		if contributes == nil {
			contributes = map[string]json.RawMessage{}
		}
		raw["contributes"], _ = json.Marshal(contributes)
		rest, _ := json.Marshal(raw)
		if err := json.Unmarshal(rest, &m); err != nil {
			return nil, fmt.Errorf("%s: %v", ManifestFile, err)
		}
	}
	var raw struct {
		Contributes struct {
			Toolbox json.RawMessage `json:"toolbox"`
		} `json:"contributes"`
	}
	if json.Unmarshal(keymap.StripJSONC(data), &raw) == nil && len(raw.Contributes.Toolbox) > 0 {
		var name string
		if json.Unmarshal(raw.Contributes.Toolbox, &name) == nil {
			m.LegacyToolbox = name
		}
	}
	if err := m.check(); err != nil {
		return nil, fmt.Errorf("%s: %v", ManifestFile, err)
	}
	return &m, nil
}

// missingToolboxString reports whether err is the old string form of
// contributes.toolbox, which the first pass cannot read as an object.
func missingToolboxString(err error) bool {
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr) && typeErr.Field == "contributes.toolbox" && typeErr.Value == "string"
}

// ReadManifest reads the manifest of the plugin in dir.
func ReadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

func (m *Manifest) checkVersion(version string) error {
	if m.FiveLeaf != "" {
		if !Satisfies(version, m.FiveLeaf) {
			return fmt.Errorf("it needs FiveLeaf %s, and this is %s", m.FiveLeaf, version)
		}
		return nil
	}
	if _, release := parseVersion(version); release && !Satisfies(mayoiCompatibilityVersion, m.Mayoi) {
		return fmt.Errorf("it needs Mayoi %s, and FiveLeaf is based on Mayoi %s", m.Mayoi, mayoiCompatibilityVersion)
	}
	return nil
}

func (m *Manifest) check() error {
	if err := ValidName(m.Name); err != nil {
		return err
	}
	if m.Main != "" && (filepath.IsAbs(m.Main) || strings.HasPrefix(filepath.Clean(m.Main), "..")) {
		return fmt.Errorf("main %q must be a path inside the plugin's folder", m.Main)
	}
	if m.Version != "" {
		if _, ok := ParseSemver(m.Version); !ok {
			return fmt.Errorf("version %q is not a version such as \"1.2.3\"", m.Version)
		}
	}
	if m.Category != "" && !ValidCategory(m.Category) {
		return fmt.Errorf("category %q is not one of %s", m.Category, strings.Join(Categories, ", "))
	}
	if m.FiveLeaf != "" && !versionRangeOK(m.FiveLeaf) {
		return fmt.Errorf("fiveleaf %q is not a version range such as \">=0.1.0\"", m.FiveLeaf)
	}
	if m.Mayoi != "" && !versionRangeOK(m.Mayoi) {
		return fmt.Errorf("mayoi %q is not a version range such as \">=0.20.0\"", m.Mayoi)
	}
	own := func(kind, id string) error {
		if !strings.HasPrefix(id, m.Name+".") || len(id) == len(m.Name)+1 {
			return fmt.Errorf("%s %q must start with %q", kind, id, m.Name+".")
		}
		return nil
	}
	c := &m.Contributes
	if c.Toolbox != nil {
		if c.Toolbox.ID != "" && !toolIDRe.MatchString(c.Toolbox.ID) {
			return fmt.Errorf("toolbox id %q must be lowercase letters, digits, dots and hyphens", c.Toolbox.ID)
		}
		if c.Toolbox.Builtin != "" {
			if c.Toolbox.Builtin != "devtool" || c.Toolbox.Page != "" || !slices.Contains([]string{"time", "base64", "json", "url"}, c.Toolbox.ID) {
				return fmt.Errorf("toolbox %q: builtin devtool requires a packaged tool ID and no page", c.Toolbox.ID)
			}
		} else if !fs.ValidPath(c.Toolbox.Page) || c.Toolbox.Page == "." || path.Ext(c.Toolbox.Page) != ".html" || strings.ContainsAny(c.Toolbox.Page, "\\:\x00") {
			return fmt.Errorf("toolbox %q page %q must be a relative HTML file in the plugin's folder", c.Toolbox.ID, c.Toolbox.Page)
		}
	}
	seen := map[string]bool{}
	for _, cmd := range c.Commands {
		if err := own("command", cmd.ID); err != nil {
			return err
		}
		if cmd.Title == "" {
			return fmt.Errorf("command %q has no title", cmd.ID)
		}
		if seen[cmd.ID] {
			return fmt.Errorf("command %q is declared twice", cmd.ID)
		}
		seen[cmd.ID] = true
	}
	for _, k := range c.Keybindings {
		if k.Key == "" && k.Mac == "" {
			return fmt.Errorf("a keybinding of %q has no key", k.Command)
		}
	}
	keymapSeen := map[string]bool{}
	for _, k := range c.Keymaps {
		if err := own("keymap", k.ID); err != nil {
			return err
		}
		if strings.TrimSpace(k.Label) == "" {
			return fmt.Errorf("keymap %q has no label", k.ID)
		}
		if keymapSeen[k.ID] {
			return fmt.Errorf("keymap %q is declared twice", k.ID)
		}
		keymapSeen[k.ID] = true
		for i, b := range k.Bindings {
			if strings.TrimSpace(strings.TrimPrefix(b.Command, "-")) == "" {
				return fmt.Errorf("keymap %q binding %d has no command", k.ID, i+1)
			}
			for _, goos := range []string{"darwin", "windows", "linux"} {
				parsed := keymap.FromEntries([]keymap.Entry{b}, goos)
				if len(parsed.Errors) > 0 {
					return fmt.Errorf("keymap %q binding %d (%s): %s", k.ID, i+1, goos, parsed.Errors[0])
				}
			}
		}
	}
	for _, s := range c.StatusItems {
		if err := own("status item", s.ID); err != nil {
			return err
		}
	}
	for _, v := range c.Views {
		if err := own("view", v.ID); err != nil {
			return err
		}
	}
	for _, p := range c.Previews {
		if err := own("preview", p.ID); err != nil {
			return err
		}
		switch p.Output {
		case "svg", "markdown", "view":
		default:
			return fmt.Errorf("preview %q: output must be svg, markdown or view", p.ID)
		}
		if p.Pattern == "" {
			return fmt.Errorf("preview %q has no pattern", p.ID)
		}
	}
	checkSettingArgs := func(id string, args map[string]string) error {
		for key, arg := range args {
			if _, ok := c.Settings[key]; !ok || !strings.Contains(arg, "${value}") {
				return fmt.Errorf("%s: settingArgs[%q] must reference a declared setting and contain ${value}", id, key)
			}
		}
		return nil
	}
	for _, f := range c.Formatters {
		if err := own("formatter", f.ID); err != nil {
			return err
		}
		if (f.Command != "" || len(f.Args) > 0 || len(f.SettingArgs) > 0) && (strings.TrimSpace(f.Command) == "" || len(f.Languages) == 0) {
			return fmt.Errorf("formatter %q needs an executable and at least one language", f.ID)
		}
		if err := checkSettingArgs(f.ID, f.SettingArgs); err != nil {
			return err
		}
	}
	for _, w := range c.Webviews {
		if err := own("webview", w.ID); err != nil {
			return err
		}
	}
	themeSeen := map[string]bool{}
	for _, t := range c.Themes {
		if err := own("theme", t.ID); err != nil {
			return err
		}
		if t.Label == "" {
			return fmt.Errorf("theme %q has no label", t.ID)
		}
		validPath := func(name string) bool {
			return fs.ValidPath(name) && name != "." && !strings.ContainsAny(name, "\\:\x00")
		}
		if !validPath(t.Path) {
			return fmt.Errorf("theme %q path %q must be inside the plugin's folder", t.ID, t.Path)
		}
		if t.DarkPath != "" && (!validPath(t.DarkPath) || t.Dark) {
			return fmt.Errorf("theme %q needs a relative darkPath and an adaptive appearance", t.ID)
		}
		if themeSeen[t.ID] {
			return fmt.Errorf("theme %q is declared twice", t.ID)
		}
		themeSeen[t.ID] = true
	}
	iconThemeSeen := map[string]bool{}
	for _, t := range c.IconThemes {
		if err := own("icon theme", t.ID); err != nil {
			return err
		}
		if strings.TrimSpace(t.Label) == "" {
			return fmt.Errorf("icon theme %q has no label", t.ID)
		}
		if !fs.ValidPath(t.Path) || t.Path == "." || strings.ContainsAny(t.Path, "\\:\x00") {
			return fmt.Errorf("icon theme %q path %q must be a relative file inside the plugin's folder", t.ID, t.Path)
		}
		if iconThemeSeen[t.ID] {
			return fmt.Errorf("icon theme %q is declared twice", t.ID)
		}
		iconThemeSeen[t.ID] = true
	}
	for _, s := range c.LanguageServers {
		if err := own("language server", s.ID); err != nil {
			return err
		}
		if s.Command == "" && (s.Resolve == nil || s.Resolve.Command == "") {
			return fmt.Errorf("language server %q has no command", s.ID)
		}
		if s.Resolve != nil && s.Command != "" {
			return fmt.Errorf("language server %q must use command or resolve, not both", s.ID)
		}
		if s.Resolve != nil && len(s.SettingArgs) > 0 {
			return fmt.Errorf("language server %q with a resolver handles settings in the resolver", s.ID)
		}
		if err := checkSettingArgs(s.ID, s.SettingArgs); err != nil {
			return err
		}
	}
	debugSeen := map[string]bool{}
	for _, adapter := range c.DebugAdapters {
		if err := own("debug adapter", adapter.ID); err != nil {
			return err
		}
		if adapter.Resolve == nil && strings.TrimSpace(adapter.Command) == "" {
			if !adapter.Transport.Direct() {
				return fmt.Errorf("debug adapter %q needs a command, a resolver or a fixed TCP address", adapter.ID)
			}
			if len(adapter.Args) > 0 || len(adapter.Env) > 0 {
				return fmt.Errorf("debug adapter %q needs a command for args or env", adapter.ID)
			}
		}
		if adapter.Resolve != nil && strings.TrimSpace(adapter.Resolve.Command) == "" {
			return fmt.Errorf("debug adapter %q needs a resolver command", adapter.ID)
		}
		if strings.ContainsAny(adapter.ID+adapter.AdapterID, " \t\r\n\x00") {
			return fmt.Errorf("debug adapter %q has an invalid id or adapterID", adapter.ID)
		}
		if len(adapter.Languages) == 0 || slices.ContainsFunc(adapter.Languages, func(language string) bool { return strings.TrimSpace(language) == "" }) {
			return fmt.Errorf("debug adapter %q needs a language ID", adapter.ID)
		}
		if len(adapter.Launch) > 0 && adapter.Launch[0] != '{' {
			return fmt.Errorf("debug adapter %q launch must be an object", adapter.ID)
		}
		if len(adapter.Attach) > 0 && adapter.Attach[0] != '{' {
			return fmt.Errorf("debug adapter %q attach must be an object", adapter.ID)
		}
		if err := adapter.Transport.Validate(); err != nil {
			return fmt.Errorf("debug adapter %q: %w", adapter.ID, err)
		}
		if debugSeen[adapter.ID] {
			return fmt.Errorf("debug adapter %q is declared twice", adapter.ID)
		}
		debugSeen[adapter.ID] = true
	}
	agentServersSeen := map[string]bool{}
	for _, server := range c.AgentServers {
		if err := own("agent server", server.ID); err != nil {
			return err
		}
		if strings.ContainsAny(server.ID, " \t\r\n\x00") {
			return fmt.Errorf("agent server %q has an invalid id", server.ID)
		}
		if agentServersSeen[server.ID] {
			return fmt.Errorf("agent server %q is declared twice", server.ID)
		}
		agentServersSeen[server.ID] = true
		if strings.TrimSpace(server.Name) == "" {
			return fmt.Errorf("agent server %q needs a name", server.ID)
		}
		launch := settings.AgentServer{
			Command: server.Command, Args: server.Args, Env: server.Env,
			Protocol: server.Protocol, WorkingDirectory: server.WorkingDirectory, Capabilities: server.Capabilities,
			Disabled: server.Disabled, TimeoutSeconds: server.TimeoutSeconds, PromptArgument: server.PromptArgument,
		}
		if err := launch.Validate(); err != nil {
			return fmt.Errorf("agent server %q: %w", server.ID, err)
		}
		if server.LocalCommand != "" {
			if err := (settings.AgentServer{Command: server.LocalCommand}).Validate(); err != nil {
				return fmt.Errorf("agent server %q localCommand: %w", server.ID, err)
			}
		}
		if key := server.LocalCommandEnv; key != "" && (server.LocalCommand == "" || strings.ContainsAny(key, "= \t\r\n\x00")) {
			return fmt.Errorf("agent server %q: localCommandEnv needs a localCommand and a valid variable name", server.ID)
		}
	}
	for _, ag := range c.Agents {
		if err := own("agent", ag.ID); err != nil {
			return err
		}
		if ag.Name == "" || ag.Resume == "" {
			return fmt.Errorf("agent %q needs a name and a resume command", ag.ID)
		}
		for _, c := range ag.Tint {
			if c != "" && !hexRe.MatchString(c) {
				return fmt.Errorf("agent %q: tint %q is not a hex color", ag.ID, c)
			}
		}
		if f := ag.Sessions; f != nil {
			if f.Glob == "" {
				return fmt.Errorf("agent %q: sessions has no glob", ag.ID)
			}
			if f.Format != "" && f.Format != "json" && f.Format != "jsonl" {
				return fmt.Errorf("agent %q: sessions format must be json or jsonl", ag.ID)
			}
		} else if !m.HasCode() {
			return fmt.Errorf("agent %q has neither sessions nor a main to list them", ag.ID)
		}
	}
	for _, l := range c.TerminalLinks {
		if _, err := regexp.Compile(l.Pattern); err != nil {
			return fmt.Errorf("terminal link %q: %v", l.Pattern, err)
		}
	}
	for key := range c.Settings {
		if key == "enabled" {
			return errors.New(`setting "enabled" is FiveLeaf's own`)
		}
	}
	return nil
}

// HasCode reports whether the plugin runs code, rather than only
// declaring keymaps, color or icon themes, languages or snippets.
func (m *Manifest) HasCode() bool { return m.Main != "" }

// DefaultOn reports whether the plugin is on while its settings leave
// "enabled" out.
func (m *Manifest) DefaultOn() bool { return m.Enabled == nil || *m.Enabled }

// Categories are the kinds a plugin can say it is, in the order the
// marketplace lists them. A plugin belongs to one; tags are search words.
var Categories = []string{
	"languages",
	"themes",
	"icon-themes",
	"agents",
	"formatters",
	"keymaps",
	"snippets",
	"debug-adapters",
	"mcp-servers",
	"tools",
}

// ValidCategory reports whether id is one of Categories.
func ValidCategory(id string) bool {
	return slices.Contains(Categories, id)
}

// Semver is a plugin version. The marketplace orders releases by it, and
// a plugin that changes what it adds raises its major number.
type Semver struct {
	Major, Minor, Patch int
	Pre                 string
}

func (v Semver) String() string {
	out := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		out += "-" + v.Pre
	}
	return out
}

// ParseSemver reads a version as major.minor.patch, with an optional
// prerelease and build metadata, as "1.2.3" or "1.2.3-beta.1+build".
func ParseSemver(v string) (Semver, bool) {
	var out Semver
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		out.Pre = v[i+1:]
		v = v[:i]
		if out.Pre == "" {
			return out, false
		}
		for _, part := range strings.Split(out.Pre, ".") {
			if part == "" {
				return out, false
			}
		}
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	var numbers [3]int
	for i, p := range parts {
		n := parseDigits(p)
		if n < 0 {
			return out, false
		}
		numbers[i] = n
	}
	out.Major, out.Minor, out.Patch = numbers[0], numbers[1], numbers[2]
	return out, true
}

// parseDigits is -1 for a string that is not a number without leading
// zeroes, so that ParseSemver compares it against the canonical form.
func parseDigits(p string) int {
	if len(p) > 1 && p[0] == '0' {
		return -1
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return -1
		}
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return -1
	}
	return n
}

// CompareSemver orders versions as semver does: prereleases before the
// release they lead to.
func CompareSemver(a, b Semver) int {
	if c := cmp.Compare(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Patch, b.Patch); c != 0 {
		return c
	}
	switch {
	case a.Pre == "" && b.Pre == "":
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	as, bs := strings.Split(a.Pre, "."), strings.Split(b.Pre, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			return cmp.Compare(an, bn)
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		default:
			return strings.Compare(as[i], bs[i])
		}
	}
	return cmp.Compare(len(as), len(bs))
}

// versionRangeOK reports whether r is a range of versions this package
// understands: comparisons of versions, separated by spaces.
func versionRangeOK(r string) bool {
	for _, part := range strings.Fields(r) {
		op := strings.TrimLeft(part, "<>=^~")
		if _, ok := parseVersion(op); !ok {
			return false
		}
	}
	return strings.TrimSpace(r) != ""
}

// parseVersion reads major.minor.patch, with the minor and patch optional.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n := 0
		if p == "" {
			return out, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return out, false
			}
			n = n*10 + int(r-'0')
		}
		out[i] = n
	}
	return out, true
}

func compareVersions(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Satisfies reports whether version is in range r; an empty range, or a
// version that does not parse (a development build), satisfies any.
func Satisfies(version, r string) bool {
	v, ok := parseVersion(version)
	if !ok || strings.TrimSpace(r) == "" {
		return true
	}
	for _, part := range strings.Fields(r) {
		op := part[:len(part)-len(strings.TrimLeft(part, "<>=^~"))]
		w, ok := parseVersion(part[len(op):])
		if !ok {
			return false
		}
		c := compareVersions(v, w)
		switch op {
		case ">=":
			ok = c >= 0
		case ">":
			ok = c > 0
		case "<=":
			ok = c <= 0
		case "<":
			ok = c < 0
		case "^":
			ok = c >= 0 && v[0] == w[0] && (w[0] != 0 || v[1] == w[1])
		case "~":
			ok = c >= 0 && v[0] == w[0] && v[1] == w[1]
		default:
			ok = c == 0
		}
		if !ok {
			return false
		}
	}
	return true
}
