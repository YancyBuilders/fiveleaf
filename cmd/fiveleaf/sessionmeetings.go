package main

import (
	"cmp"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
)

const meetingHistorySource agents.Harness = "roundtable"

func sessionInProject(session agents.Session, root string) bool {
	if session.Harness == meetingHistorySource {
		return sameSessionDirectory(root, session.Dir)
	}
	return session.In(root)
}

func (a *app) meetingProjectFolders() []string {
	folders := slices.Clone(a.settings.Recent)
	folders = slices.AppendSeq(folders, maps.Keys(a.settings.Sessions))
	for _, project := range a.projects() {
		folders = append(folders, project.folder())
	}
	return folders
}

func withMeetingHistory(sessions []agents.Session, dataDir string, folders []string) ([]agents.Session, map[string]roundtable.Library, error) {
	libraries, problems := roundtable.Libraries(dataDir, folders)
	byKey := make(map[string]roundtable.Library, len(libraries))
	chatCount := len(sessions)
	for _, library := range libraries {
		groups, err := library.List()
		problems = errors.Join(problems, err)
		key := library.Key()
		byKey[key] = library
		for _, group := range groups {
			names := make([]string, 0, len(group.Members))
			for _, member := range group.Members {
				names = append(names, member.Name)
			}
			sessions = append(sessions, agents.Session{Harness: meetingHistorySource, Root: key, Dir: library.Project,
				ID: group.ID, Title: group.Topic, Prompt: strings.Join(names, " · "), Updated: group.Updated})
		}
	}
	if len(sessions) != chatCount {
		slices.SortStableFunc(sessions, func(left, right agents.Session) int {
			if order := right.Updated.Compare(left.Updated); order != 0 {
				return order
			}
			return cmp.Compare(left.Key(), right.Key())
		})
	}
	return sessions, byKey, problems
}

func (a *app) resumeMeeting(record agents.Session) {
	library, found := a.sessions.meetingLibraries[record.Root]
	if !found {
		a.fail("%s", locales.SessionsContinueFailed(os.ErrNotExist.Error()))
		return
	}
	if !library.Owns(library.Project) {
		a.openMeetingRecord(library, record.ID)
		return
	}
	a.inSessionProject(library.Project, func(owner *app) { owner.openMeetingRecord(library, record.ID) })
}
