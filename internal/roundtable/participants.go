package roundtable

import (
	"errors"
	"path/filepath"
	"slices"
)

func (group Group) hasModeratorSlot() bool {
	return len(group.Members) == MaxMembers && slices.ContainsFunc(group.Members, func(member Participant) bool { return member.ID == ModeratorID })
}

func (group Group) CanInviteParticipant() bool {
	return len(group.Members) < MaxMembers || group.hasModeratorSlot()
}

func (room *Room) InviteParticipant(member Participant) (Group, error) {
	room.mu.Lock()
	if room.archive == nil || !room.archive.group.hasModeratorSlot() {
		room.mu.Unlock()
		return room.Invite(member)
	}
	defer room.mu.Unlock()
	if room.closed() {
		return Group{}, ErrClosed
	}
	if room.chain != nil {
		return Group{}, ErrTurnActive
	}
	group := room.archive.group
	group.Members = append(slices.Clone(group.Members), member)
	return room.archive.saveParticipants(group)
}

func (store *archive) saveParticipants(group Group) (Group, error) {
	if err := group.validate(); err != nil {
		return Group{}, err
	}
	if err := writeRecord(filepath.Join(store.dir, "group.json"), group); err != nil {
		return Group{}, err
	}
	store.group = group
	return group, nil
}

func (room *Room) RemoveParticipant(id string) (Group, error) {
	room.mu.Lock()
	defer room.mu.Unlock()
	switch {
	case room.closed():
		return Group{}, ErrClosed
	case room.chain != nil:
		return Group{}, ErrTurnActive
	case room.archive == nil:
		return Group{}, errors.New("meeting has no saved member list")
	}
	store := room.archive
	library := Library{dir: filepath.Dir(store.dir)}
	group, err := library.Group(store.group.ID)
	if err != nil {
		return Group{}, err
	}
	index := slices.IndexFunc(group.Members, func(member Participant) bool { return member.ID == id })
	if index < 0 {
		return Group{}, ErrUnknownAgent
	}
	group.Members = slices.Delete(group.Members, index, index+1)
	if id == ModeratorID {
		group.Options.HumanChair = true
	}
	if _, err := store.saveParticipants(group); err != nil {
		return Group{}, err
	}
	room.options = normalizeOptions(group.Options)
	room.members = slices.DeleteFunc(room.members, func(member Member) bool { return member.ID == id })
	room.notifyLocked()
	return group, nil
}

func (room *Room) MentionTarget(text string) string {
	if !hasLeadingMention(text) {
		return ""
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	if index := mentionedMember(room.members, text); index >= 0 {
		return room.members[index].ID
	}
	return ""
}

func (room *Room) MentionBinding(text string) (id, token string) {
	room.mu.Lock()
	defer room.mu.Unlock()
	index, token := matchMemberMention(room.members, text)
	if index >= 0 {
		return room.members[index].ID, token
	}
	return "", token
}
