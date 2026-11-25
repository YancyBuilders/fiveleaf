package main

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	agentThinkingFooterHeight float32 = 40
	agentThinkingRailInset    float32 = 12
	agentThinkingName                 = "Thinking intensity"
	agentThinkingPending              = "Changing…"
)

type agentThinkingState struct {
	value            float64
	dragging, queued bool
	pending          *string
	shown            string
	elapsed          time.Duration
	last             time.Time
	moving           bool
}

func agentThinkingModes(s *agentChat) []agentChoice {
	if s.thinkingModes {
		return s.modes
	}
	var choices []agentChoice
	for _, choice := range s.modes {
		name := strings.ToLower(strings.TrimSpace(choice.name))
		if strings.HasPrefix(name, "thinking:") || strings.HasPrefix(name, "reasoning:") {
			choices = append(choices, choice)
		}
	}
	return choices
}

func agentGenericModes(s *agentChat) []agentChoice {
	if s.thinkingModes {
		return s.genericModes
	}
	thinking := agentThinkingModes(s)
	if len(thinking) == 0 {
		return s.modes
	}
	var choices []agentChoice
	for _, choice := range s.modes {
		if !slices.ContainsFunc(thinking, func(mode agentChoice) bool { return mode.id == choice.id }) {
			choices = append(choices, choice)
		}
	}
	return choices
}

func agentGenericMode(s *agentChat) string {
	if s.thinkingModes {
		return s.genericMode
	}
	return s.mode
}

func agentThinkingLabel(choice agentChoice) string {
	switch strings.ToLower(choice.id) {
	case "auto":
		return "Auto"
	case "off":
		return "Off"
	case "minimal":
		return "Minimal"
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "high":
		return "High"
	case "xhigh":
		return "Extra high"
	case "max":
		return "Max"
	}
	if choice.name != "" {
		return choice.name
	}
	return choice.id
}

func (a *app) chooseAgentThinking(s *agentChat, id string) {
	k := &s.modelPicker
	if !a.chatShown(s) || s.client == nil || s.sessionID == "" || s.requestInFlight() || k.thinking.pending != nil || id == s.mode ||
		!slices.ContainsFunc(agentThinkingModes(s), func(choice agentChoice) bool { return choice.id == id }) {
		return
	}
	guard := agentModelPickerState{root: a.root, generation: s.generation, client: s.client,
		session: s.sessionID, target: s.agentTarget}
	pending := &id
	k.thinking.pending, k.thinking.queued = pending, true
	a.post(func() {
		state := &s.modelPicker.thinking
		if state.pending != pending {
			return
		}
		state.queued = false
		if !guard.valid(a, s) {
			state.pending = nil
			return
		}
		a.setChatMode(s, id)
		if !s.configuring {
			state.pending = nil
		}
	})
}

func (a *app) agentThinkingPickerRow(c *ui.Context, s *agentChat, choices []agentChoice) {
	p, m := a.pal, a.pal.metricsOr()
	state := &s.modelPicker.thinking
	if state.pending != nil && !state.queued && !s.configuring {
		state.pending = nil
	}
	mode := s.mode
	if state.pending != nil {
		mode = *state.pending
	}
	index := slices.IndexFunc(choices, func(choice agentChoice) bool { return choice.id == mode })
	shown := mode
	var labelWidth float32
	if index < 0 {
		labelWidth, _ = c.MeasureText(0, ui.Span{Text: agentThinkingLabel(agentChoice{id: mode}), Size: m.textSmall})
	}
	for _, choice := range choices {
		shown += "\x00" + choice.id
		width, _ := c.MeasureText(0, ui.Span{Text: agentThinkingLabel(choice), Size: m.textSmall})
		labelWidth = max(labelWidth, width)
	}
	if state.shown != shown {
		state.value, state.shown = float64(max(0, index)), shown
	}
	unavailable := s.client == nil || s.sessionID == "" || s.busy || s.connecting || len(choices) < 2
	disabled := unavailable || s.configuring || state.pending != nil
	ui.Column(c.Key("agent-thinking-footer")).Height(agentThinkingFooterHeight).FillWidth().MinWidth(0).Shrink(0).Children(func() {
		ui.Box(c).Height(1).Shrink(0).Background(p.border)
		ui.Row(c).Grow(1).FillWidth().MinWidth(0).Padding(8, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconSparkles).Size(16, 16).Shrink(0).TextColor(p.muted).Tooltip(agentThinkingName)
			track := ui.Box(c.Key("agent-thinking-track")).Height(24).Grow(1).MinWidth(0)
			var rail ui.Element
			track.Children(func() {
				rail = ui.SliderBase(c.Key("agent-thinking"), &state.value, 0, float64(max(1, len(choices)-1))).Step(1).
					Label(agentThinkingName).Absolute().Top(0).Left(0).Fill().Padding(0, agentThinkingRailInset).Radius(12).
					Disabled(disabled).FocusRing(false)
			})
			if unavailable {
				track.Opacity(0.5)
			} else {
				rail.Cursor(ui.CursorPointer)
			}
			if s.modelPicker.focus {
				rail.Focus()
				s.modelPicker.focus = false
			}
			changed := rail.Changed()
			_, _, held := rail.Dragged()
			released := state.dragging && !held
			state.dragging = held
			selected := min(len(choices)-1, max(0, int(math.Round(state.value))))
			choice := choices[selected]
			displayChoice, displayIndex := choice, selected
			if index < 0 && !changed && !held && !released {
				// An adapter may report a current value before publishing its
				// options. Show that value rather than claiming the first is set.
				displayChoice, displayIndex = agentChoice{id: mode, name: mode}, -1
			}
			label := agentThinkingLabel(displayChoice)
			value := label
			if state.pending != nil {
				value += " · " + agentThinkingPending
			}
			rail.Value(value)
			still := c.Preferences().ReduceMotion || a.blurred || displayIndex < 0
			rtl := c.Direction() == ui.RightToLeft
			position := float32(displayIndex)
			if travel := rail.LayoutBounds().W - 2*agentThinkingRailInset; held && !disabled && travel > 0 {
				x, _, _ := rail.PointerPosition()
				fraction := clamp((x-agentThinkingRailInset)/travel, 0, 1)
				if rtl {
					fraction = 1 - fraction
				}
				position = rail.Jump("fill", fraction*float32(len(choices)-1))
			} else if still || len(choices) == 1 {
				rail.Jump("fill", max(0, position))
			} else {
				position = rail.AnimateWith("fill", position, motionTap, ui.EaseInOut)
			}
			track.Draw(func(pt *ui.Painter, r ui.Rect) {
				state.advance(pt.Now(), still)
				if state.moving {
					pt.AnimationFrameRate(float32(time.Second) / float32(motionAgentThinkingFrame))
				}
				paintAgentThinkingRail(pt, r, p, choices, position, state.elapsed, rail.FocusVisible(), rtl)
			})
			ui.Text(c, label).Role(ui.RoleNone).FontSize(m.textSmall).TextColor(p.text).SingleLine().
				Width(float32(math.Ceil(float64(labelWidth)))).MaxWidthPercent(35).MinWidth(0).Shrink(0).TextAlign(ui.End)
			if !disabled && !state.dragging && (changed || released) {
				a.chooseAgentThinking(s, choice.id)
			}
		})
	})
}

func (s *agentThinkingState) advance(now time.Time, still bool) {
	if !still && s.moving && !s.last.IsZero() {
		if delta := now.Sub(s.last); delta > 0 {
			s.elapsed = (s.elapsed + min(delta, 4*motionAgentThinkingFrame)) % motionAgentThinkingCycle
		}
	}
	s.last, s.moving = now, !still
}

func agentThinkingColors(level string, dark bool) (ui.Color, ui.Color) {
	var from, to ui.Color
	switch strings.ToLower(level) {
	case "off", "none":
		from, to = ui.Hex("#32a9c9"), ui.Hex("#72e7e4")
	case "minimal":
		from, to = ui.Hex("#7578dc"), ui.Hex("#b0a7f4")
	case "low":
		from, to = ui.Hex("#7462ee"), ui.Hex("#c49cff")
	case "medium":
		from, to = ui.Hex("#9560e5"), ui.Hex("#e19ae9")
	case "high":
		from, to = ui.Hex("#aa54de"), ui.Hex("#f591ce")
	case "xhigh":
		from, to = ui.Hex("#df6b9d"), ui.Hex("#ffbd83")
	case "max", "ultra":
		from, to = ui.Hex("#ff8f3d"), ui.Hex("#ffd76f")
	default:
		from, to = ui.Hex("#7e8492"), ui.Hex("#c1c4cc")
	}
	if !dark {
		from, to = from.Mix(white, 0.12), to.Mix(white, 0.14)
	}
	return from, to
}

func paintAgentThinkingRail(pt *ui.Painter, r ui.Rect, p *palette, choices []agentChoice, position float32, elapsed time.Duration, focused, rtl bool) {
	count := len(choices)
	if r.W <= 0 || r.H <= 0 || count == 0 {
		return
	}
	inset := min(agentThinkingRailInset, r.W/2)
	travel := r.W - 2*inset
	pt.Fill(r, p.hover, r.H/2)
	fill := r
	position = min(position, float32(count-1))
	if position < 0 {
		fill.W = 0
	} else if count > 1 {
		fill.W = 2*inset + travel*position/float32(count-1)
	}
	if rtl {
		fill.X += r.W - fill.W
	}
	phase := float64(elapsed) / float64(motionAgentThinkingCycle) * 2 * math.Pi
	selected := int(math.Round(float64(position)))
	pt.Clip(fill, r.H/2, func() {
		if fill.W <= 0 {
			return
		}
		lower := int(position)
		from, to := agentThinkingColors(choices[lower].id, p.dark)
		if blend := position - float32(lower); blend > 0 {
			nextFrom, nextTo := agentThinkingColors(choices[lower+1].id, p.dark)
			from, to = from.Mix(nextFrom, blend), to.Mix(nextTo, blend)
		}
		pt.FillGradient(fill, ui.LinearGradient{From: from, To: to, Angle: 0}, r.H/2)
		// A few continuous, broad waves and fine highlights make the liquid
		// flow visible without a bitmap upload or a background render loop.
		for band := range 3 {
			var points [33][2]float32
			for i := range points {
				x := float64(i) / float64(len(points)-1)
				y := 0.24 + float64(band)*0.25 + 0.16*math.Sin(x*math.Pi*3+phase+float64(band)*2) +
					0.06*math.Sin(x*math.Pi*7-phase*2)
				points[i] = [2]float32{r.X + float32(x)*r.W, r.Y + float32(y)*r.H}
			}
			pt.StrokePolyline(points[:], 8, white.Alpha(0.08))
			pt.StrokePolyline(points[:], 2.5, white.Alpha(0.13))
			pt.StrokePolyline(points[:], 0.75, white.Alpha(0.36))
		}
		if level := choices[selected].id; level == "max" || level == "ultra" || level == "xhigh" {
			for i := range 17 {
				x := math.Mod(float64(i)*0.137+phase/(2*math.Pi)*0.2, 1)
				y := 0.16 + 0.70*math.Mod(float64(i)*0.379, 1) + 0.07*math.Sin(phase*2+float64(i))
				size := float32(1.5 + 0.8*math.Sin(float64(i)))
				pt.Fill(ui.Rect{X: r.X + float32(x)*r.W, Y: r.Y + float32(y)*r.H, W: size, H: size},
					white.Alpha(0.70), size/2)
			}
		}
	})
	for i := range count {
		offset := inset + float32(i)*travel/float32(max(1, count-1))
		if rtl {
			offset = r.W - offset
		}
		color := p.muted.Alpha(0.65)
		if position >= 0 && i <= selected {
			color = white.Alpha(0.78)
		}
		pt.Fill(ui.Rect{X: r.X + offset - 1.25, Y: r.Y + r.H - 5.5, W: 2.5, H: 2.5}, color, 1.25)
	}
	border := p.border
	if focused {
		border = p.accent
	}
	pt.Stroke(r, border, r.H/2, 1)
}
