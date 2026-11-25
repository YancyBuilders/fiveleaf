package roundtable

import (
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxTopicRunes = 120

var errMemberLimit = errors.New("meeting member limit reached")

type Participant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type Group struct {
	ID           string        `json:"id"`
	Topic        string        `json:"topic"`
	Members      []Participant `json:"members"`
	Created      time.Time     `json:"created"`
	Updated      time.Time     `json:"-"`
	Options      Options       `json:"options"`
	SystemPrompt string        `json:"systemPrompt,omitempty"`
}

type Library struct {
	dir     string
	Project string
}

func projectKey(root string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(root))))
}

func (lib Library) Key() string { return filepath.Base(lib.dir) }

func (lib Library) Path() string { return lib.dir }

func (lib Library) Owns(project string) bool {
	root, err := projectRoot(project)
	return err == nil && projectKey(root) == lib.Key()
}

func projectRoot(project string) (string, error) {
	if !filepath.IsAbs(project) {
		return "", errors.New("a local project folder is required")
	}
	root, err := filepath.EvalSymlinks(project)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a project folder: %s", root)
	}
	return root, nil
}

func ProjectLibrary(dataDir, project string) (Library, error) {
	if dataDir == "" {
		return Library{}, errors.New("an application data directory is required")
	}
	root, err := projectRoot(project)
	if err != nil {
		return Library{}, err
	}
	return Library{dir: filepath.Join(dataDir, "roundtables", projectKey(root)), Project: root}, nil
}

func Libraries(dataDir string, knownProjects []string) ([]Library, error) {
	if dataDir == "" {
		return nil, nil
	}
	dir := filepath.Join(dataDir, "roundtables")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	known := make(map[string]string)
	for _, project := range knownProjects {
		if lib, err := ProjectLibrary(dataDir, project); err == nil {
			known[lib.Key()] = lib.Project
		}
	}
	var libraries []Library
	var problems error
	for _, entry := range entries {
		key := entry.Name()
		if !entry.IsDir() || len(key) != sha256.Size*2 || strings.Trim(key, "0123456789abcdef") != "" {
			continue
		}
		lib := Library{dir: filepath.Join(dir, key), Project: known[key]}
		if lib.Project == "" {
			raw, err := os.ReadFile(filepath.Join(lib.dir, "project.json"))
			var project string
			switch {
			case err == nil:
				if json.Unmarshal(raw, &project) == nil && filepath.IsAbs(project) && projectKey(project) == key {
					lib.Project = project
				} else {
					problems = errors.Join(problems, fmt.Errorf("invalid roundtable project in %s", lib.dir))
				}
			case !errors.Is(err, os.ErrNotExist):
				problems = errors.Join(problems, err)
			}
		}
		libraries = append(libraries, lib)
	}
	return libraries, problems
}

func ValidTopic(text string) bool {
	text = strings.TrimSpace(text)
	return text != "" && utf8.ValidString(text) && utf8.RuneCountInString(text) <= MaxTopicRunes && strings.IndexFunc(text, unicode.IsControl) < 0
}

func validGroupID(id string) bool {
	return len(id) >= 26 && len(id) <= 64 && strings.Trim(id, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") == ""
}

func (group Group) HasModerator() bool {
	return slices.ContainsFunc(group.Members, func(member Participant) bool { return member.ID == ModeratorID })
}

func (group Group) validate() error {
	limit := MaxMembers
	if group.HasModerator() {
		limit++
	}
	if !validGroupID(group.ID) || !ValidTopic(group.Topic) || len(group.Members) == 0 && !group.Options.HumanChair || len(group.Members) > limit || len(group.SystemPrompt) > MaxSystemPromptBytes {
		return errors.New("invalid roundtable topic or invitations")
	}
	seen := make(map[string]bool, len(group.Members))
	for _, member := range group.Members {
		id := strings.ToLower(member.ID)
		if seen[id] || strings.TrimSpace(id) == "" || strings.TrimSpace(member.Name) == "" || member.Provider == "" || len(member.ID)+len(member.Name)+len(member.Provider) > 4096 {
			return errors.New("invalid or duplicate roundtable member")
		}
		seen[id] = true
	}
	return nil
}

func writeRecord(path string, value any) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".pending-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(value)
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (lib Library) Create(topic string, members []Participant) (Group, error) {
	return lib.CreateWithChair(topic, members, false)
}

func (lib Library) CreateWithChair(topic string, members []Participant, human bool) (Group, error) {
	now := time.Now()
	options := Options{ContextMessages: DefaultMemoryWindow, HumanChair: human}
	if slices.ContainsFunc(members, func(member Participant) bool { return member.ID == ModeratorID }) {
		options.MaxHops = MaxTasksPerTurn
	}
	group := Group{ID: rand.Text(), Topic: strings.TrimSpace(topic), Members: slices.Clone(members), Created: now, Updated: now, Options: normalizeOptions(options)}
	if err := group.validate(); err != nil {
		return Group{}, err
	}
	if err := os.MkdirAll(lib.dir, 0o700); err != nil {
		return Group{}, err
	}
	if err := writeRecord(filepath.Join(lib.dir, "project.json"), lib.Project); err != nil {
		return Group{}, err
	}
	dir := filepath.Join(lib.dir, group.ID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return Group{}, err
	}
	if err := writeRecord(filepath.Join(dir, "group.json"), group); err != nil {
		os.Remove(dir)
		return Group{}, err
	}
	return group, nil
}

func (lib Library) Group(id string) (Group, error) {
	var group Group
	if !validGroupID(id) {
		return group, errors.New("invalid roundtable ID")
	}
	dir := filepath.Join(lib.dir, id)
	raw, err := os.ReadFile(filepath.Join(dir, "group.json"))
	if err != nil {
		return group, err
	}
	if err := json.Unmarshal(raw, &group); err != nil {
		return Group{}, err
	}
	if group.ID != id {
		return Group{}, errors.New("roundtable ID does not match its directory")
	}
	group.Updated = group.Created
	if info, err := os.Stat(filepath.Join(dir, "messages")); err == nil {
		group.Updated = info.ModTime()
	} else if !errors.Is(err, os.ErrNotExist) {
		return Group{}, err
	}
	return group, group.validate()
}

func (lib Library) Delete(id string) error {
	if lib.dir == "" || !validGroupID(id) {
		return errors.New("invalid roundtable archive")
	}
	root, err := os.OpenRoot(lib.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()

	pending := ".deleted-" + id
	info, err := root.Lstat(id)
	if err == nil {
		if !info.IsDir() {
			return errors.New("roundtable archive is not a directory")
		}
		err = root.Rename(id, pending)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return root.RemoveAll(pending)
}

func (lib Library) EnsureModerator(id string, moderator Participant) (Group, error) {
	return lib.bindModerator(id, moderator, false)
}

func (lib Library) bindModerator(id string, moderator Participant, takeChair bool) (Group, error) {
	group, err := lib.Group(id)
	if err != nil {
		return Group{}, err
	}
	if moderator.ID != ModeratorID {
		return Group{}, errors.New("invalid moderator")
	}
	if group.Options.HumanChair && !takeChair {
		return group, nil
	}
	index := slices.IndexFunc(group.Members, func(member Participant) bool { return member.ID == ModeratorID })
	if index >= 0 {
		if !takeChair || !group.Options.HumanChair {
			return group, nil
		}
		if group.Members[index].Provider != moderator.Provider {
			return Group{}, ErrModeratorUnavailable
		}
	} else {
		group.Members = append(group.Members, moderator)
	}
	group.Options.HumanChair = false
	if err := group.validate(); err != nil {
		return Group{}, err
	}
	return group, writeRecord(filepath.Join(lib.dir, id, "group.json"), group)
}

func (room *Room) BindModerator(moderator Participant, takeChair bool, bind func(Group) []Member) error {
	room.mu.Lock()
	defer room.mu.Unlock()
	switch {
	case room.closed():
		return ErrClosed
	case takeChair && room.chain != nil:
		return ErrTurnActive
	case room.archive == nil:
		return ErrModeratorUnavailable
	}
	store := room.archive
	group, err := store.library.bindModerator(store.group.ID, moderator, takeChair)
	if err != nil {
		return err
	}
	store.group = group
	room.options = normalizeOptions(group.Options)
	room.setMembersLocked(cloneMembers(bind(group)))
	return nil
}

func (lib Library) List() ([]Group, error) {
	entries, err := os.ReadDir(lib.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var groups []Group
	var problems error
	for _, entry := range entries {
		if !entry.IsDir() || !validGroupID(entry.Name()) {
			continue
		}
		group, err := lib.Group(entry.Name())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if _, err := os.Lstat(filepath.Join(lib.dir, entry.Name())); errors.Is(err, os.ErrNotExist) {
					continue
				}
			}
			problems = errors.Join(problems, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		groups = append(groups, group)
	}
	slices.SortFunc(groups, func(left, right Group) int {
		return cmp.Or(right.Updated.Compare(left.Updated), cmp.Compare(left.ID, right.ID))
	})
	return groups, problems
}

type archive struct {
	dir     string
	group   Group
	library Library
	filesMu sync.Mutex
	files   []string
}

func messageFile(message Message) string {
	return message.Time.UTC().Format("20060102T150405.000000000") + "-" + message.ID + ".json"
}

func (store *archive) append(message Message) error {
	dir := filepath.Join(store.dir, "messages")
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return writeRecord(filepath.Join(dir, messageFile(message)), message)
}

func (store *archive) page(before string, limit int) ([]Message, bool, error) {
	return store.selectPage(before, limit, nil)
}

func (store *archive) filesBefore(before string) (older []string, found bool, err error) {
	store.filesMu.Lock()
	defer store.filesMu.Unlock()
	if before == "" || len(store.files) == 0 || before > store.files[len(store.files)-1] {
		entries, err := os.ReadDir(filepath.Join(store.dir, "messages"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, false, err
		}
		files := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
				files = append(files, entry.Name())
			}
		}
		store.files = files
	}
	if before == "" {
		return store.files, false, nil
	}
	end, found := slices.BinarySearch(store.files, before)
	return store.files[:end], found, nil
}

func (store *archive) selectPage(before string, limit int, include func(Message) bool) ([]Message, bool, error) {
	files, _, err := store.filesBefore(before)
	if err != nil {
		return nil, false, err
	}
	return store.readPage(files, limit, include)
}

func (store *archive) readPage(files []string, limit int, include func(Message) bool) ([]Message, bool, error) {
	dir := filepath.Join(store.dir, "messages")
	var page []Message
	var size int
	index := len(files) - 1
	for ; index >= 0 && len(page) < limit; index-- {
		message, err := readArchivedMessage(dir, files[index])
		if err != nil {
			return nil, false, err
		}
		if include != nil && !include(message) {
			continue
		}
		size += messageSize(message)
		if size > MaxHistoryBytes && len(page) > 0 {
			break
		}
		page = append(page, message)
	}
	slices.Reverse(page)
	return page, index >= 0, nil
}

func readArchivedMessage(dir, name string) (Message, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return Message{}, err
	}
	var message Message
	if err := json.Unmarshal(raw, &message); err != nil {
		return Message{}, fmt.Errorf("%s: %w", name, err)
	}
	if messageFile(message) != name || len(message.Text) > MaxMessageBytes {
		return Message{}, errors.New("invalid roundtable message: " + name)
	}
	if message.Role == "assistant" && message.Task == nil {
		if clean := legacyPiAnswer(message.Text); clean != message.Text {
			message.Original, message.Text = message.Text, clean
		}
	}
	if message.Task != nil && message.Task.Status == Running {
		message.Task.Status = Stopped
	}
	return message, nil
}

func (store *archive) configure(options Options, system string) error {
	group := store.group
	group.Options, group.SystemPrompt = options, system
	if err := writeRecord(filepath.Join(store.dir, "group.json"), group); err != nil {
		return err
	}
	store.group = group
	return nil
}

func (room *Room) Invite(member Participant) (Group, error) {
	room.mu.Lock()
	defer room.mu.Unlock()
	if room.closed() {
		return Group{}, ErrClosed
	}
	if room.archive == nil {
		return Group{}, errors.New("meeting has no saved member list")
	}
	group := room.archive.group
	if len(group.Members) >= MaxMembers {
		return Group{}, errMemberLimit
	}
	group.Members = append(slices.Clone(group.Members), member)
	if err := group.validate(); err != nil {
		return Group{}, err
	}
	if err := writeRecord(filepath.Join(room.archive.dir, "group.json"), group); err != nil {
		return Group{}, err
	}
	room.archive.group = group
	return group, nil
}

func (store *archive) clear(memory memoryRecord) error {
	dir := filepath.Join(store.dir, "messages")
	old := filepath.Join(store.dir, ".cleared-"+rand.Text())
	renamed := os.Rename(dir, old)
	if renamed != nil && !errors.Is(renamed, os.ErrNotExist) {
		return renamed
	}
	if err := writeRecord(filepath.Join(store.dir, "memory.json"), memory); err != nil {
		if renamed == nil {
			return errors.Join(err, os.Rename(old, dir))
		}
		return err
	}
	if renamed == nil {
		os.RemoveAll(old)
	}
	store.filesMu.Lock()
	store.files = nil
	store.filesMu.Unlock()
	return nil
}

func (lib Library) OpenRoom(group Group, members []Member) (*Room, error) {
	if err := group.validate(); err != nil {
		return nil, err
	}
	store := &archive{dir: filepath.Join(lib.dir, group.ID), group: group, library: lib}
	if _, err := os.Stat(filepath.Join(store.dir, "group.json")); err != nil {
		return nil, err
	}
	messages, earlier, err := store.page("", MaxHistoryMessages)
	if err != nil {
		return nil, err
	}
	room := newRoom(members, group.Options)
	room.archive, room.messages, room.earlier, room.system = store, messages, earlier, group.SystemPrompt
	if err := room.loadMemory(); err != nil {
		return nil, err
	}
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "assistant" {
			if last := slices.IndexFunc(room.members, func(member Member) bool { return member.ID == messages[index].AgentID }); last >= 0 {
				room.next = (last + 1) % len(room.members)
			}
			break
		}
	}
	go room.run()
	return room, nil
}

func (room *Room) Earlier(before Message, limit int) ([]Message, bool, error) {
	if room.archive == nil {
		return nil, false, nil
	}
	return room.archive.page(messageFile(before), min(max(1, limit), MaxHistoryMessages))
}

func (room *Room) EarlierOverview(before Message, limit int) ([]Message, bool, error) {
	if room.archive == nil {
		return nil, false, nil
	}
	return room.archive.selectPage(messageFile(before), min(max(1, limit), MaxHistoryMessages), Message.InOverview)
}

type OverviewPage struct {
	Messages, TaskContext []Message
	HasEarlier            bool
}

func (room *Room) EarlierOverviewPage(before Message, limit int) (OverviewPage, error) {
	store := room.archive
	if store == nil {
		return OverviewPage{}, nil
	}
	cursor := messageFile(before)
	files, found, err := store.filesBefore(cursor)
	if err != nil {
		return OverviewPage{}, err
	}
	messages, more, err := store.readPage(files, min(max(1, limit), MaxHistoryMessages), Message.InOverview)
	if err != nil || len(messages) == 0 {
		return OverviewPage{}, err
	}
	prefix, err := room.EarlierTurnPrefix(messages[0])
	if err != nil {
		return OverviewPage{}, err
	}
	details := slices.Clip(messages)
	if len(prefix) > 0 {
		details = append(prefix, messages...)
	}
	last := messages[len(messages)-1]
	adjacent := found && files[len(files)-1] == messageFile(last)
	if task := last.Task; adjacent && last.Role == "assistant" && task != nil && task.Stage == Coordinate && task.Status == Completed && !task.NoAnswer {
		next, err := readArchivedMessage(filepath.Join(store.dir, "messages"), cursor)
		if err != nil {
			return OverviewPage{}, err
		}
		if next.Task != nil && next.Task.Stage == Work && next.Task.TurnID == task.TurnID && next.Hop == last.Hop+1 && next.AgentID != "" {
			details = append(details, next)
		}
	}
	return OverviewPage{Messages: messages, TaskContext: details, HasEarlier: more}, nil
}

func (room *Room) EarlierTurnPrefix(first Message) ([]Message, error) {
	if room.archive == nil || first.Task == nil || first.Hop == 1 {
		return nil, nil
	}
	legacy := first.Task.TurnID == ""
	if legacy && (first.Hop < 2 || first.AgentID == "") {
		return nil, nil
	}
	files, found, err := room.archive.filesBefore(messageFile(first))
	if err != nil || !found {
		return nil, err
	}
	dir := filepath.Join(room.archive.dir, "messages")
	var prefix []Message
	for index := len(files) - 1; index >= 0 && len(prefix) < 2*MaxTasksPerTurn; index-- {
		message, err := readArchivedMessage(dir, files[index])
		if err != nil {
			return nil, err
		}
		if message.Role != "assistant" && message.Role != "task" {
			break
		}
		if message.Task == nil || message.Task.TurnID != first.Task.TurnID || message.Task.Stage == Conclusion {
			break
		}
		if legacy && (message.Hop != first.Hop-len(prefix)-1 || message.AgentID == "") {
			break
		}
		message.Original = ""
		prefix = append(prefix, message)
		if message.Hop == 1 {
			break
		}
	}
	slices.Reverse(prefix)
	return prefix, nil
}
