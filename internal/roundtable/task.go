package roundtable

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ModeratorID = "preset:moderate"
	Coordinate  = "coordinate"
	Work        = "work"
	Conclusion  = "conclusion"
	Direct      = "direct"
	Running     = "running"
	Completed   = "completed"
	Failed      = "failed"
	Stopped     = "stopped"
)

type Task struct {
	TurnID      string
	Assignment  string
	Stage       string
	Status      string
	Finished    time.Time
	Tools       []ToolRecord `json:",omitempty"`
	NoAnswer    bool         `json:",omitempty"`
	MemoryNotes int          `json:",omitempty"`
	MemoryTurns int          `json:",omitempty"`
}

type ToolRecord struct {
	ID, Kind, Title, Status, Detail string
	Note                            string `json:",omitempty"`
	InputOnly                       bool   `json:",omitempty"`
}

type discussion struct {
	turnID, topic, goal, assignment, next, stage string
	remaining                                    int
	humanChair                                   bool
	notes                                        []string
	roster                                       []Member
}

func specialist(member Member) bool {
	return member.ID != ModeratorID && available(member)
}

func exactMember(members []Member, id string) (Member, bool) {
	for _, member := range members {
		if member.ID == id {
			return member, available(member)
		}
	}
	return Member{}, false
}

func (d *discussion) dispatchStage(members []Member) string {
	if d.stage == Coordinate && (d.remaining == 0 || !slices.ContainsFunc(members, specialist)) {
		return Conclusion
	}
	return d.stage
}

func (d *discussion) brief(stage string, members []Member) string {
	var text strings.Builder
	switch stage {
	case Coordinate:
		text.WriteString("You are the moderator. Decide who can supply the next missing piece, or give the final answer now. To delegate, reply with one line: @exact-member-ID followed by a space and one specific assignment. Select one available member from this list. You may assign the same member again. To conclude, give a stand-alone answer based on the available evidence.\nAvailable members:\n")
		for _, member := range members {
			if specialist(member) {
				fmt.Fprintf(&text, "- @%s (%q)", member.ID, member.Name)
				if len(member.Capabilities) > 0 {
					fmt.Fprintf(&text, " %q", boundedText(strings.Join(member.Capabilities, ", "), 256))
				}
				text.WriteByte('\n')
			}
		}
	case Work:
		text.WriteString("Report your concrete finding and its evidence to the moderator. The moderator decides the next assignment and writes the conclusion. Return only your finding.\n")
	case Conclusion:
		text.WriteString("You are the moderator and have the closing assignment. Synthesize the available findings into a stand-alone answer to the user's objective. Identify any unresolved question or missing evidence. Return only the final answer. No handoff.\n")
	case Direct:
		if d.humanChair {
			text.WriteString("The human chairs this meeting and has given you the floor for one reply. Answer their assignment directly. The human selects the next speaker and publishes the meeting conclusion.\n")
		} else {
			text.WriteString("Answer the user directly as the requested member. Hand off with a leading @exact-member-ID only when explicitly necessary.\n")
		}
	}
	text.WriteString("Use the user's language. In a public answer, put the decision in the first sentence. Use up to three short supporting points unless the user requests more. Include complete requested diagrams or examples. Skip meeting recaps, startup information, skill inventories and tool logs. When asked for analysis or a plan, do not change files.\n")
	fmt.Fprintf(&text, "Meeting topic: %s\nUser's objective: %s\n", d.topic, boundedText(d.goal, 16<<10))
	if stage == Work || stage == Direct {
		fmt.Fprintf(&text, "Your assignment: %s\n", boundedText(d.assignment, 4096))
	}
	for _, note := range d.notes {
		fmt.Fprintf(&text, "Previous member's finding: %s\n", note)
	}
	return text.String()
}

type taskContextKey struct{}

type taskReporter struct {
	room *Room
	id   string
}

func TaskID(ctx context.Context) string {
	reporter, _ := ctx.Value(taskContextKey{}).(taskReporter)
	return reporter.id
}

func ReportTool(ctx context.Context, tool ToolRecord) {
	reporter, ok := ctx.Value(taskContextKey{}).(taskReporter)
	if !ok || tool.ID == "" || ctx.Err() != nil {
		return
	}
	room := reporter.room
	room.mu.Lock()
	defer room.mu.Unlock()
	if room.chain == nil || room.chain.task.ID != reporter.id {
		return
	}
	tool.ID = boundedText(tool.ID, 256)
	tool.Kind = boundedText(tool.Kind, 64)
	tool.Status = boundedText(tool.Status, 64)
	tool.Title = boundedText(tool.Title, 512)
	tool.Detail = boundedText(tool.Detail, 4096)
	tool.Note = boundedText(tool.Note, 1024)
	task := *room.chain.task.Task
	index := slices.IndexFunc(task.Tools, func(old ToolRecord) bool { return old.ID == tool.ID })
	if index < 0 && len(task.Tools) >= 64 {
		return
	}
	task.Tools = slices.Clone(task.Tools)
	if index < 0 {
		task.Tools = append(task.Tools, tool)
	} else {
		task.Tools[index] = tool
	}
	room.chain.task.Task = &task
	if room.archive != nil && (tool.Status == "completed" || tool.Status == "failed") {
		select {
		case room.checkpoint <- struct{}{}:
		default:
		}
	}
}

func (r *Room) saveProgress() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed() || r.chain == nil || r.chain.task.Task == nil {
		return
	}
	if err := r.persistLocked(r.chain.task); err != nil {
		r.err = err.Error()
	}
	r.notifyLocked()
}

func taskSize(task *Task) int {
	if task == nil {
		return 0
	}
	size := len(task.TurnID) + len(task.Assignment) + len(task.Stage) + len(task.Status)
	for _, tool := range task.Tools {
		size += len(tool.ID) + len(tool.Kind) + len(tool.Title) + len(tool.Status) + len(tool.Detail) + len(tool.Note)
	}
	return size
}

func (message Message) InOverview() bool {
	if message.Task == nil {
		return message.Role == "user" || message.Role == "assistant" && message.Text != ""
	}
	return message.Role == "assistant" || message.Task.Status != Completed
}

func (r *Room) finishTaskLocked(status, text string) bool {
	message := r.chain.task
	if message.Task == nil {
		return true
	}
	r.chain.task = Message{}
	finished := *message.Task
	finished.Status, finished.Finished = status, time.Now()
	message.Task, message.Text = &finished, text
	if status == Completed {
		message.Role = "assistant"
	}
	var err error
	if r.archive != nil {
		err = r.archive.append(message)
		if err == nil {
			r.queueConclusionDayLocked(message)
		}
	}
	r.appendLocked(message)
	if err != nil {
		r.err = fmt.Sprintf("saving roundtable task: %v", err)
	}
	return err == nil
}

func moderatorAssignment(members []Member, first, body string) (Member, string, error) {
	var target Member
	var assignment string
	for _, member := range members {
		rest, match := strings.CutPrefix(first, "@"+member.ID)
		separator, _ := utf8.DecodeRuneInString(rest)
		if match && len(member.ID) > len(target.ID) && (rest == "" || unicode.IsSpace(separator)) {
			target, assignment = member, strings.TrimSpace(rest)
		}
	}
	if !specialist(target) {
		return Member{}, "", ErrUnknownAgent
	}
	assignment = strings.TrimSpace(assignment + "\n" + body)
	if assignment == "" || len(assignment) > 4096 || slices.ContainsFunc(strings.Split(assignment, "\n"), hasLeadingMention) {
		return Member{}, "", ErrInvalidAssignment
	}
	return target, assignment, nil
}

func (r *Room) failTaskLocked(err error, output string) {
	r.err = err.Error()
	text := r.err
	if output != "" {
		text += "\n\n" + boundedText(output, MaxMessageBytes-len(text)-2)
	}
	r.finishTaskLocked(Failed, text)
	r.stopLocked()
}

func (r *Room) acceptTaskLocked(answer string) {
	current := r.chain
	work := current.discussion
	task := *current.task.Task
	answer = strings.TrimSpace(legacyPiAnswer(answer))
	first, body, _ := strings.Cut(answer, "\n")
	body = strings.TrimSpace(body)
	if strings.TrimSpace(first) == "FINAL" {
		answer = body
	}
	if answer == "" && !(task.Stage == Direct && task.NoAnswer) {
		r.failTaskLocked(ErrNoAnswer, "")
		return
	}
	finished := false
	switch task.Stage {
	case Coordinate, Conclusion:
		if !hasLeadingMention(answer) {
			task.Stage, finished = Conclusion, true
			break
		}
		if task.Stage == Conclusion {
			r.failTaskLocked(ErrConclusionRequired, answer)
			return
		}
		recipient, assignment, err := moderatorAssignment(work.roster, strings.TrimSpace(first), body)
		if err != nil {
			r.failTaskLocked(err, answer)
			return
		}
		if _, ok := exactMember(r.members, recipient.ID); !ok {
			r.failTaskLocked(ErrUnknownAgent, answer)
			return
		}
		work.next, work.stage, work.assignment = recipient.ID, Work, assignment
		work.remaining--
	case Work:
		if hasLeadingMention(first) {
			answer = body
		}
		if answer == "" {
			r.failTaskLocked(ErrNoAnswer, strings.TrimSpace(first))
			return
		}
		work.notes = append(work.notes, fmt.Sprintf("%s (@%s), %s: %s", current.task.Name, current.task.AgentID, boundedText(task.Assignment, 512), boundedText(answer, 3<<10)))
		work.next, work.stage, work.assignment = ModeratorID, Coordinate, work.goal
	case Direct:
		if work.remaining == 0 || !hasLeadingMention(first) {
			finished = true
			break
		}
		index := mentionedMember(r.members, first)
		if index < 0 {
			r.failTaskLocked(ErrUnknownAgent, answer)
			return
		}
		recipient := r.members[index]
		work.next, work.assignment = recipient.ID, work.goal
		for _, alias := range []string{recipient.ID, recipient.Name} {
			end := 1 + len(alias)
			if end <= len(first) && strings.EqualFold(first[1:end], alias) {
				if assignment := strings.TrimSpace(strings.TrimLeft(first[end:], " :：,，")); assignment != "" {
					work.assignment = boundedText(assignment, 4096)
				}
				break
			}
		}
		if body != "" {
			work.notes = append(work.notes, current.task.Name+": "+boundedText(body, 3<<10))
		}
		task.Stage = Work
		work.remaining--
	}
	current.task.Task = &task
	if !r.finishTaskLocked(Completed, answer) || finished {
		r.stopLocked()
		return
	}
	current.inflight, r.active = false, nil
	r.scheduleLocked()
}
