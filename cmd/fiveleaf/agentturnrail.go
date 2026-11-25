package main

import (
	"math"
	"strconv"
	"strings"

	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

const (
	agentTurnRailWidth = 52
	agentTurnStep      = 10
)

type agentTurnAnchor struct {
	prompt, reply *agentMessage
	offset        float32
}

func (chat *agentChat) readTurn(turn agentTurnAnchor) {
	target := min(chat.scroll.MaxY, max(0, turn.offset))
	if chat.readingPrompt != turn.prompt || math.Abs(float64(target-chat.scroll.Y)) > 0.5 {
		chat.scroll.Y = target
	}
	chat.readingPrompt, chat.readingY = turn.prompt, chat.scroll.Y
}

type agentTurnRailState struct {
	active, hovered, focused int
	keyboard                 bool
	preview                  *agentMessage
	card                     ui.Handle
}

func (a *app) agentTurnRail(c *ui.Context, chat *agentChat, turns []agentTurnAnchor, view ui.Rect) {
	active := 0
	atEnd := chat.scroll.Y >= chat.scroll.MaxY-2
	for index, turn := range turns {
		if turn.prompt == chat.readingPrompt {
			chat.readTurn(turn)
			active = index
			break
		}
		if atEnd || turn.offset <= chat.scroll.Y+1 {
			active = index
		}
	}
	preview := -1
	var anchor ui.Rect
	gutter := ui.Column(c.Key("turn-gutter")).Absolute().Left(0).Top(4).Bottom(4).Width(agentTurnRailWidth).
		Justify(ui.Center).PassThrough()
	state := ui.Local(gutter, "browse", func() agentTurnRailState { return agentTurnRailState{active: -1, focused: -1, hovered: -1} })
	gutter.Children(func() {
		rail := ui.Scroll(c.Key("turns")).FillWidth().Height(float32(len(turns)) * agentTurnStep).MaxHeightPercent(100).
			MinHeight(0).FocusGroup(ui.Vertical).Role(ui.RoleToolbar).Label(locales.AgentTurnNavigation())
		rail.Children(func() {
			hovered, focused := -1, -1
			overCard := false
			if state.preview != nil {
				pointerX, pointerY, inside := c.Root().PointerPosition()
				overCard = inside && state.card.Bounds(c).Contains(pointerX, pointerY)
			}
			buttons := make([]ui.Element, len(turns))
			for index, turn := range turns {
				mark := ui.ButtonBase(c.Key(index)).FillWidth().Height(agentTurnStep).Padding(0, 0, 0, 16).
					Justify(ui.Start).Radius(3).Cursor(ui.CursorPointer).Checked(index == active).
					Label(locales.AgentTurnNumber(strconv.Itoa(index + 1))).Description(turn.prompt.questionTitle())
				buttons[index] = mark
				if overCard && turn.prompt == state.preview {
					preview = index
				}
				if mark.Hovered() {
					hovered = index
				}
				if mark.FocusVisible() {
					focused = index
				}
				if index == active && active != state.active {
					mark.ScrollIntoView()
				}
				if mark.Clicked() {
					chat.readTurn(turn)
				}
			}
			if hovered != state.hovered {
				state.keyboard = false
			}
			if focused >= 0 && focused != state.focused {
				state.keyboard = true
			}
			state.active, state.hovered, state.focused = active, hovered, focused
			if hovered >= 0 {
				preview = hovered
			}
			if focused >= 0 && (state.keyboard || preview < 0) {
				preview = focused
			}
			if a.blurred || a.overlaid() {
				preview = -1
			}
			if preview >= 0 {
				anchor = buttons[preview].Bounds()
			}
			top := chat.scroll.Y - agentTranscriptInset
			for index, mark := range buttons {
				width := float32(6)
				color := a.pal.faint.Alpha(0.38)
				switch {
				case preview >= 0:
					distance := max(index-preview, preview-index)
					width += float32(5 * max(0, 4-distance))
					if index == preview {
						color = a.pal.text
					}
				case index == active:
					color = a.pal.text.Alpha(0.72)
				case turns[index].offset < top+view.H && (index+1 == len(turns) || turns[index+1].offset > top):
					color = a.pal.muted.Alpha(0.78)
				}
				width = mark.Animate("length", width, motionHover)
				mark.Children(func() { ui.Box(c).Size(width, 2).Radius(1).Background(color).Transition(hoverFade) })
			}
		})
	})
	if preview >= 0 {
		state.preview = turns[preview].prompt
		a.agentTurnPreview(c, turns[preview], preview+1, anchor, view, &state.card)
	} else {
		state.preview = nil
	}
}

func (a *app) agentTurnPreview(c *ui.Context, turn agentTurnAnchor, number int, anchor, view ui.Rect, handle *ui.Handle) {
	metrics := a.pal.metricsOr()
	left := float32(agentTurnRailWidth)
	width := min(320, view.W-left-metrics.gap)
	gutterRoom := (view.W-agentConversationWidth)/2 + agentTranscriptInset - left - metrics.gap
	if gutterRoom >= 240 {
		width = min(width, gutterRoom)
	}
	height := min(112, view.H-2*metrics.gap)
	if width <= 0 || height <= 0 {
		return
	}
	top := max(metrics.gap, min(anchor.Y+anchor.H/2-view.Y-height/2, view.H-height-metrics.gap))
	ui.Column(c.Key("turn-preview-anchor")).Absolute().Left(left).Top(top).Size(width, height).
		Justify(ui.Center).PassThrough().Children(func() {
		panel := ui.Column(c.Key("turn-preview")).Bind(handle).FillWidth().MaxHeightPercent(100).Padding(metrics.inset).
			Gap(metrics.pad).Radius(metrics.radiusCard).Clip().Role(ui.RoleTooltip).Label(locales.AgentTurnPreview())
		a.floating(c, panel, false).Children(func() {
			title := turn.prompt.questionTitle()
			if title == "" {
				title = locales.AgentTurnNumber(strconv.Itoa(number))
			}
			ui.Text(c, title).SingleLine().FillWidth().FontSize(metrics.textTitle).FontWeight(metrics.weightBold).TextColor(a.pal.text)
			if turn.reply != nil {
				excerpt, clipped := agentVisibleText(strings.TrimSpace(turn.reply.text), 768)
				excerpt = strings.Join(strings.Fields(excerpt), " ")
				if clipped {
					excerpt += "…"
				}
				ui.Text(c, excerpt).FillWidth().MinWidth(0).MaxLines(3).FontSize(metrics.textBody).LineHeight(1.45).TextColor(a.pal.muted)
			}
		})
	})
}
