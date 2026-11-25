package main

import (
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/remote"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type roundtableView uint8

const (
	roundtableViewChat roundtableView = iota
	roundtableViewGraph
)

type roundtableWindow struct {
	a                           *app
	meeting                     *agentMeeting
	tab                         *colTab
	pal                         *palette
	closed                      bool
	sessionsSidebar             bool
	media                       *roundtableMediaPreview
	inspector                   meetingInspector
	drawer                      meetingDrawerKind
	drawerFocus                 bool
	drawerGraph                 roundtable.Graph
	showFullReplies, focusDraft bool
	view                        roundtableView
	graphScroll                 ui.ScrollState
	graphSel                    string
	graphMatrixPage             int
	taskReveal                  bool
	pinsExpanded                bool

	scroll, membersScroll              ui.ScrollState
	input                              ui.Handle
	transcriptContent                  ui.Handle
	draft, target                      string
	err                                string
	lastUserMessage                    string
	mentions                           meetingMentionPicker
	creating, focusTopic, confirmClear bool
	createHumanChair                   bool
	topic, brief, templateBrief        string
	selected                           map[string]roundtable.Participant
	drafts                             map[string]meetingDraft
	viewGeneration                     uint64
	inviteScroll                       ui.ScrollState
	page                               []roundtable.Message
	pageCursors                        []roundtable.Message
	pageGraph                          roundtable.Graph
	moreBefore                         bool
	memberID, expandedTask, openTool   string
	pinnedTask                         *roundtable.Message
	taskScroll                         ui.ScrollState
	olderTasks                         []roundtable.Message
	oldestTaskPage                     roundtable.Message
	taskPageEnd, loadingTasks          bool
	taskVisit                          uint64
}

func newRoundtableWindow(a *app, meeting *agentMeeting) *roundtableWindow {
	w := &roundtableWindow{a: a, drafts: map[string]meetingDraft{}}
	if meeting != nil {
		w.enter(meeting)
	} else {
		w.newGroup()
	}
	return w
}

func (a *app) openRoundtable() {
	if a.window.Detached {
		a.host.inWorkspace((*app).openRoundtable)
		return
	}
	if a.root == "" {
		return
	}
	if remote.Is(a.root) {
		a.fail("%s", locales.AgentRemoteUnavailable())
		return
	}
	if a.meetingWindow != nil {
		a.showRoundtable(nil)
		return
	}
	if a.meeting == nil {
		library, err := a.roundtableLibrary()
		if err == nil {
			var groups []roundtable.Group
			groups, err = library.List()
			if len(groups) != 0 {
				a.openMeetingRecord(library, groups[0].ID)
				return
			}
		}
		if err != nil {
			a.fail("%s", locales.SessionsContinueFailed(err.Error()))
		}
	}
	a.showRoundtable(a.meeting)
}

func (a *app) showRoundtable(meeting *agentMeeting) {
	w := a.meetingWindow
	if w == nil {
		w = newRoundtableWindow(a, meeting)
		a.meetingWindow = w
	} else if meeting != nil {
		w.enter(meeting)
	}
	w.sessionsSidebar = false
	if col, at := a.locate(w.tab); col != nil {
		a.activate(col, at, true)
		return
	}
	col := a.column()
	at := col.active + 1
	w.tab = &colTab{tool: &toolView{kind: toolRoundtable, meeting: w}}
	a.addTab(col, at, w.tab)
	a.activate(col, at, true)
}

func (a *app) activeRoundtableTab() *roundtableWindow {
	if a.meetingWindow != nil && a.meetingWindow.tab != nil && a.activeTab() == a.meetingWindow.tab {
		return a.meetingWindow
	}
	return nil
}

func (a *app) closeRoundtable() {
	if w := a.meetingWindow; w != nil {
		if w.tab != nil {
			a.dropTab(w.tab)
		}
		w.finish()
	}
}

func (w *roundtableWindow) finish() {
	if w.closed {
		return
	}
	w.closed = true
	if w.a.meetingWindow == w {
		w.a.closeMeeting()
		w.a.meetingWindow = nil
	}
}

func (w *roundtableWindow) post(fn func()) {
	generation := w.viewGeneration
	w.postWindow(func() {
		if generation == w.viewGeneration {
			fn()
		}
	})
}

func (w *roundtableWindow) postWindow(fn func()) {
	w.a.post(func() {
		if !w.closed && !w.a.loadsClosed {
			fn()
		}
	})
}

func (w *roundtableWindow) changeMeeting(kind meetingChange, change func(*roundtable.Room) error, complete func(*agentMeeting, error)) {
	w.post(func() {
		meeting := w.meeting
		if w.creating || meeting == nil || meeting.readOnly || meeting.changing != nil {
			return
		}
		changed := make(chan struct{})
		meeting.changing, meeting.change = changed, kind
		bg.Go(func() {
			err := change(meeting.room)
			close(changed)
			w.a.post(func() {
				meeting.changing, meeting.change = nil, meetingMemoryChange
				if !w.a.loadsClosed {
					complete(meeting, err)
					w.a.releaseFinishedMeeting(meeting)
				}
			})
		})
	})
}

func (w *roundtableWindow) render(c *ui.Context) {
	w.pal = w.a.pal
	shell := ui.Column(c.Key("roundtable").Key(w.viewGeneration)).Fill().MinWidth(0).MinHeight(0).Clip()
	width := shell.Bounds().W
	wide := width >= 672
	shell.Children(func() {
		if w.creating || w.meeting == nil {
			w.createView(c)
			return
		}
		snapshot := w.meeting.room.Snapshot()
		graph := roundtable.BuildGraph(snapshot)
		panelVisible := func() bool { return w.inspector.visible(width > 1040 && w.view == roundtableViewChat) }
		w.header(c, snapshot, panelVisible(), width)
		w.graphTabs(c, graph)
		panel := panelVisible()
		detailGraph := graph
		if w.view == roundtableViewChat {
			switch {
			case w.page != nil:
				detailGraph = w.historyGraph()
			default:
				if prefix := w.liveTurnPrefix(c, shell, snapshot); len(prefix) != 0 {
					context := snapshot
					context.Messages = slices.Concat(prefix, snapshot.Messages)
					detailGraph = roundtable.BuildTaskGraph(context)
				} else if snapshot.ActiveTask.ID != "" || slices.ContainsFunc(snapshot.Messages, func(message roundtable.Message) bool {
					return message.Task != nil && (message.Role == "task" || message.Task.NoAnswer || message.ToolOnly)
				}) {
					detailGraph = roundtable.BuildTaskGraph(snapshot)
				}
			}
		}
		ui.Row(c.Key("meeting-body")).FillWidth().Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if wide || !panel {
				ui.Column(c.Key("meeting-chat")).Grow(1).MinWidth(0).MinHeight(0).Background(materialOf(c).canvasColor(mix(w.pal.editor, w.pal.panel, 0.64))).Children(func() {
					if w.view == roundtableViewGraph {
						graphWidth := width
						if graphWidth <= 0 {
							graphWidth, _ = c.Size()
						}
						if panel {
							graphWidth -= w.inspectorWidth(width)
						}
						w.graphView(c, graph, graphWidth)
						return
					}
					w.transcript(c, snapshot, detailGraph)
					if w.meeting.readOnly {
						ui.Text(c, locales.SessionsMeetingReadOnly()).FontSize(12).TextColor(w.pal.muted).Padding(10, 14)
					} else {
						w.meetingApprovals(c)
						w.composer(c, snapshot)
					}
				})
			}
			if panel {
				w.memberInspector(c, snapshot, graph, width, wide)
			}
		})
		if w.view == roundtableViewChat && panel && !wide && !w.meeting.readOnly {
			w.meetingApprovals(c)
		}
		w.meetingDrawer(c, snapshot, detailGraph)
	})
	w.mediaOverlay(c)
}

func (w *roundtableWindow) liveTurnPrefix(c *ui.Context, shell ui.Element, snapshot roundtable.Snapshot) []roundtable.Message {
	if !snapshot.HasEarlier || len(snapshot.Messages) == 0 {
		return nil
	}
	first := snapshot.Messages[0]
	if first.Task == nil || first.Hop <= 1 {
		return nil
	}
	type prefixState struct {
		first, failure string
		messages       []roundtable.Message
		retryAfter     time.Time
	}
	cache := ui.Local(shell, "first-turn", func() prefixState { return prefixState{} })
	if cache.first == first.ID && !cache.retryAfter.IsZero() {
		if wait := cache.retryAfter.Sub(c.Now()); wait > 0 {
			c.After(wait)
			return nil
		}
		cache.first = ""
	}
	if cache.first != first.ID {
		cache.first, cache.messages = first.ID, nil
		cache.retryAfter = time.Time{}
		started := c.Now()
		room, generation := w.meeting.room, w.viewGeneration
		go func() {
			prior, err := room.EarlierTurnPrefix(first)
			w.postWindow(func() {
				if w.viewGeneration != generation || cache.first != first.ID {
					return
				}
				if err != nil {
					cache.failure = err.Error()
					w.err = cache.failure
					cache.retryAfter = started.Add(time.Second)
					return
				}
				if cache.failure != "" && w.err == cache.failure {
					w.err = ""
				}
				cache.failure = ""
				cache.messages = prior
			})
		}()
	}
	return cache.messages
}

func (w *roundtableWindow) meetingApprovals(c *ui.Context) {
	if chat := w.meeting.chat; chat != nil && len(chat.permissions) > 0 {
		ui.Scroll(c.Key("meeting-approvals")).FillWidth().MaxHeight(164).Children(func() { w.a.agentPermissionsView(c, chat, w.pal) })
	}
}

func (w *roundtableWindow) graphTabs(c *ui.Context, graph roundtable.Graph) {
	p := w.pal
	bar := ui.Row(c.Key("meeting-views")).FillWidth().MinWidth(0).Height(36).Shrink(0).AlignItems(ui.Stretch).Padding(0, 14).Gap(12).BorderWidth(0, 0, 1, 0).BorderColor(p.border)
	bar.Children(func() {
		for index, label := range []string{locales.RoundtableChatView(), locales.RoundtableGraphView()} {
			mode := roundtableView(index)
			selected := w.view == mode
			button := ui.ButtonBase(c.Key("meeting-view").Key(index)).Label(label).Role(ui.RoleTab).Checked(selected).KeepFocus().AlignItems(ui.Center).Padding(0, 3).Shrink(0).Cursor(ui.CursorPointer)
			ink := p.muted
			if selected {
				ink = p.accent
				button.BorderWidth(0, 0, 2, 0).BorderColor(ink)
			} else if button.Hovered() || button.FocusVisible() {
				button.Background(p.hover)
			}
			button.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(ink).SingleLine() })
			if button.Clicked() && !selected {
				w.view = mode
				w.inspector.switchView(mode)
				w.selectMember("")
			}
		}
		ui.Spacer(c).MinWidth(0)
		ui.Row(c.Key("graph-counts")).MinWidth(0).Gap(5).AlignItems(ui.Center).Children(func() {
			for index, label := range []string{locales.RoundtableGraphCountsNodes(len(graph.Nodes)), locales.RoundtableGraphCountsHandoffs(graph.Handoffs())} {
				ui.Text(c.Key(index), label).FontSize(10.5).FontFeatures("tnum").TextColor(p.muted).Background(p.muted.Alpha(0.09)).Radius(5).Padding(2, 5).MinWidth(0).SingleLine().Tooltip(label)
			}
		})
	})
}

func (w *roundtableWindow) header(c *ui.Context, snapshot roundtable.Snapshot, panel bool, width float32) {
	p := w.pal
	ui.Row(c.Key("roundtable-header")).Height(58).Shrink(0).FillWidth().Padding(0, 14).Gap(7).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
		home := ui.ButtonBase(c.Key("group-home")).MinWidth(0).Grow(1).AlignItems(ui.Center).Gap(10).Justify(ui.Start).Label(locales.RoundtableOverview()).Cursor(ui.CursorPointer)
		home.Children(func() {
			ui.Icon(c, iconMessageText).FontSize(18).TextColor(p.muted).Shrink(0)
			ui.Column(c).MinWidth(0).Grow(1).Gap(2).Children(func() {
				ui.Text(c, w.meeting.group.Topic).FontSize(15).FontWeight(600).TextColor(p.text).SingleLine().MinWidth(0).Tooltip(w.meeting.group.Topic)
				ui.Row(c.Key("group-summary")).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
					w.chairBadge(c, snapshot.HumanChair)
					ui.Text(c, locales.RoundtablePeople(len(w.meeting.speakers))).FontSize(10.5).TextColor(p.muted).SingleLine().MinWidth(0)
				})
			})
		})
		if home.Clicked() {
			w.returnToChat()
		}
		if snapshot.Busy {
			ui.Spinner(c).Size(13, 13)
			if !w.meeting.readOnly && ui.Button(c, locales.AgentStop()).FontSize(12).Clicked() {
				w.post(w.meeting.room.Stop)
			}
		}
		if roundtableIconButton(c, "export", iconDownload, locales.RoundtableExport(), p).Clicked() {
			w.post(w.exportMeeting)
		}
		if w.headerAction(c, "group-memory", meetingMemoryIcon, locales.RoundtableGroupMemory(), locales.RoundtableOpenMemory(), width).Clicked() {
			w.openMeetingMemory()
		}
		if w.headerAction(c, "collaboration", iconSettings, locales.RoundtableCollaboration(), locales.RoundtableCollaboration(), width).Clicked() {
			w.openDrawer(meetingDrawerCollaboration)
		}
		if roundtableIconButton(c, "member-panel", iconSidebarRight, locales.RoundtableMemberPanel(), p).Expanded(panel).Checked(panel).Clicked() {
			w.inspector.setVisible(!panel)
		}
	})
}

func (w *roundtableWindow) headerAction(c *ui.Context, key string, icon *ui.SVG, title, label string, width float32) ui.Element {
	if width < 640 {
		return roundtableIconButton(c, key, icon, label, w.pal)
	}
	return actionButton(c.Key(key), title, icon).Label(label).Tooltip(label).Shrink(0)
}

func (w *roundtableWindow) confirmClearMeeting(c *ui.Context) {
	ui.Row(c.Key("confirm-clear")).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.RoundtableClearConfirm()).FontSize(12).TextColor(w.pal.text).FillWidth()
		if ui.Button(c, locales.ButtonCancel()).Clicked() {
			w.confirmClear = false
		}
		if ui.Button(c, locales.LogClear()).Disabled(w.meeting.changing != nil).Clicked() {
			w.confirmClear = false
			w.changeMeeting(meetingClear, (*roundtable.Room).Clear, func(meeting *agentMeeting, err error) {
				if err == nil && w.a.ownsMeeting(meeting) {
					w.a.disconnectChat(meeting.chat)
					meeting.chat = &agentChat{}
					meeting.chatTask, meeting.reviews = "", nil
				}
				if w.closed || w.creating || w.meeting != meeting {
					return
				}
				if err != nil {
					w.err = err.Error()
					return
				}
				w.lastUserMessage, w.graphSel, w.pinsExpanded = "", "", false
				w.viewGeneration++
				w.pageGraph = roundtable.Graph{}
				w.page, w.pageCursors, w.err = nil, nil, ""
				w.selectMember("")
				w.closeDrawer()
			})
		}
	})
}

func (w *roundtableWindow) transcript(c *ui.Context, snapshot roundtable.Snapshot, graph roundtable.Graph) {
	p := w.pal
	snapshot.Messages = w.historyPage(c, snapshot)
	follow := w.page == nil && w.scroll.Y >= w.scroll.MaxY-2
	if w.page == nil {
		for index := len(snapshot.Messages) - 1; index >= 0; index-- {
			message := snapshot.Messages[index]
			if message.Role != "user" {
				continue
			}
			if follow && message.ID != w.lastUserMessage {
				w.scroll.Y = math.MaxFloat32
			}
			w.lastUserMessage = message.ID
			break
		}
	}
	micaChatViewport(c, &w.transcriptContent, p, &w.a.settings.Prefs, func() {
		ui.Scroll(c.Key("roundtable-transcript")).TrackScroll(&w.scroll).FillWidth().MinWidth(0).Grow(1).MinHeight(0).
			Background(materialOf(c).canvasColor(meetingCanvas(p))).Padding(14, 16, 8, 16).Children(func() {
			ui.Column(c).Bind(&w.transcriptContent).FillWidth().MinWidth(0).MaxWidth(930).AlignSelf(ui.Center).Gap(16).Children(func() {
				if w.meeting.room != nil {
					w.pinnedMemoryStrip(c, w.meeting.room.Memory().Notes)
				}
				if len(snapshot.Messages) == 0 {
					ui.Text(c, locales.RoundtableEmpty()).FontSize(12.5).TextColor(p.muted).LineHeight(1.5)
				}
				w.messageFlow(c, snapshot, graph, w.page == nil)
				if snapshot.Error != "" {
					text, _ := meetingExcerpt(snapshot.Error)
					ui.Text(c, text).FontSize(12).TextColor(p.dangerFg).Selectable()
				}
				if w.err != "" {
					ui.Text(c, w.err).FontSize(12).TextColor(p.dangerFg).Selectable()
				}
			})
		})
	})
}

func meetingReadingPreview(text string) string {
	if strings.Count(text, "\n") <= 16 && (len(text) <= 900 || utf8.RuneCountInString(text) <= 900) {
		return ""
	}
	doc := parser.Parse(text)
	var preview strings.Builder
	for index := 1; index < doc.Len(); index++ {
		block := doc.Block(index)
		if block.Kind != parser.Paragraph {
			continue
		}
		line := strings.TrimSpace(parser.PlainText(doc.Inlines(index)))
		if line == "" {
			continue
		}
		if preview.Len() > 0 {
			preview.WriteString("\n")
		}
		part, clipped := meetingExcerpt(line)
		preview.WriteString(part)
		if clipped || utf8.RuneCountInString(preview.String()) >= 180 {
			break
		}
	}
	return preview.String()
}

func (w *roundtableWindow) composer(c *ui.Context, snapshot roundtable.Snapshot) {
	material := materialOf(c)
	bottom := float32(12)
	if material.mica() {
		bottom = 0
	}
	ui.Column(c.Key("roundtable-composer")).FillWidth().MinWidth(0).Shrink(0).Background(material.canvasColor(meetingCanvas(w.pal))).Padding(4, 16, bottom, 16).Children(func() {
		w.composeBox(c, snapshot)
	})
}

func (w *roundtableWindow) composeBox(c *ui.Context, snapshot roundtable.Snapshot) {
	p := w.pal
	metrics := p.metricsOr()
	clearing := w.meeting.change == meetingClear
	box := w.a.chatComposeBox(c.Key("draft-box")).MaxWidth(930).AlignSelf(ui.Center).Shrink(0)
	box.Children(func() {
		placeholder := locales.RoundtablePlaceholder()
		if snapshot.HumanChair && w.target == "" {
			placeholder = locales.RoundtableHumanChairDraft()
		}
		input := w.a.chatDraftInput(c.Key("roundtable-input"), &w.draft, &w.input, placeholder).Disabled(clearing)
		if w.focusDraft && !clearing {
			input.Focus()
			w.focusDraft = false
		}
		files := ui.Local(box, "file-picker", func() meetingFilePicker { return meetingFilePicker{} })
		if !files.open && !clearing {
			w.mentionPicker(c, input)
		}
		ui.Row(c.Key("compose-tools")).FillWidth().MinWidth(0).Gap(metrics.gap).Padding(0, metrics.inset, metrics.pad, metrics.inset).AlignItems(ui.Center).Children(func() {
			if meetingComposeAction(c, "mention", locales.RoundtableMention(), meetingAtIcon, p).Disabled(clearing).Clicked() {
				w.insertMention(input)
			}
			if meetingComposeAction(c, "reference", locales.RoundtableReferenceFile(), iconFileText, p).Disabled(clearing).Clicked() {
				w.startFilePicker(input, files)
			}
			ui.Spacer(c)
			if snapshot.Busy {
				label, color := w.meetingActivity(p.accent)
				if task := snapshot.ActiveTask; task.Task != nil {
					label = task.Name + " · " + label
				}
				ui.Spinner(c).Size(10, 10).Shrink(0)
				ui.Text(c, label).FontSize(metrics.textSmall).TextColor(color).MinWidth(0).SingleLine().Tooltip(label)
			} else {
				w.postHumanConclusion(c, snapshot)
			}
			problem := w.recipientProblem(w.draft)
			if problem != "" {
				ui.Text(c, problem).FontSize(11).TextColor(p.muted).SingleLine().MinWidth(0).Tooltip(problem)
			}
			ready := w.meeting.change == meetingMemoryChange && problem == "" && strings.TrimSpace(w.draft) != ""
			if w.a.agentSendButton(c, iconArrowUp, locales.AgentSend(), ready).Disabled(!ready).Clicked() {
				w.post(w.send)
			}
		})
		if files.open && !clearing {
			w.filePicker(c, input, files)
		}
	})
}

func (w *roundtableWindow) meetingActivity(workingInk ui.Color) (string, ui.Color) {
	if chat := w.meeting.chat; chat != nil && len(chat.permissions) > 0 {
		return locales.AgentTaskApproval(), w.pal.modifiedFg
	}
	return locales.AgentWorking(), workingInk
}

func (w *roundtableWindow) send() {
	if w.meeting == nil || w.meeting.change != meetingMemoryChange {
		return
	}
	w.syncMentionTarget(w.draft)
	if problem := w.recipientProblem(w.draft); problem != "" {
		w.err = problem
		return
	}
	if err := w.meeting.room.Discuss(w.meeting.group.Topic, w.draft, w.target); err != nil {
		w.err = err.Error()
		return
	}
	w.clearSentDraft()
}

func (w *roundtableWindow) clearSentDraft() {
	w.draft, w.err, w.target, w.mentions = "", "", "", meetingMentionPicker{}
	w.page, w.pageCursors = nil, nil
	// A manually sent message should be visible even after reading older turns.
	w.scroll.Y = math.MaxFloat32
}

func roundtableIconButton(c *ui.Context, key string, icon *ui.SVG, label string, p *palette) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(28, 28).Radius(5).AlignItems(ui.Center).Justify(ui.Center).Label(label).Tooltip(label).Cursor(ui.CursorPointer).Shrink(0)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(14).TextColor(p.muted) })
	return b
}
