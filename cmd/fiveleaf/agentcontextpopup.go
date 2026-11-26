package main

import (
	"strconv"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func agentContextCompactionFraction(snapshot agentContextSnapshot) (float32, bool) {
	if !snapshot.known || !snapshot.compactionKnown || !snapshot.compactionEnabled || snapshot.size <= 0 {
		return 0, false
	}
	return float32(float64(max(0, min(snapshot.compactAt, snapshot.size))) / float64(snapshot.size)), true
}

func agentContextCompactionStatusLabel(status agentCompactionStatus) string {
	switch status {
	case agentCompactionManaged:
		return locales.AgentContextCompactionManaged()
	case agentCompactionRunning:
		return locales.AgentContextCompactionRunning()
	case agentCompactionCompleted:
		return locales.AgentContextCompactionCompleted()
	case agentCompactionFailed:
		return locales.AgentContextCompactionFailed()
	}
	return ""
}

func agentContextCategoryLabel(kind string) string {
	switch kind {
	case "system":
		return locales.AgentContextSystemPrompt()
	case "instructions":
		return locales.AgentContextInstructions()
	case "skills":
		return locales.AgentContextSkills()
	case "builtin_tools":
		return locales.AgentContextBuiltinTools()
	case "custom_tools":
		return locales.AgentContextCustomTools()
	case "user":
		return locales.AgentContextUserMessages()
	case "assistant":
		return locales.AgentContextAssistantMessages()
	case "thinking":
		return locales.AgentContextThinking()
	case "tool_calls":
		return locales.AgentContextToolCalls()
	case "tool_output":
		return locales.AgentContextToolOutput()
	default:
		return locales.AgentContextUnattributed()
	}
}

func agentContextCategoryColor(p *palette, kind string) ui.Color {
	switch kind {
	case "system", "user":
		return p.accent
	case "skills":
		return p.addedFg
	case "builtin_tools", "tool_calls":
		return p.modifiedFg
	case "custom_tools", "thinking":
		return legible(p.ansi[5], p.dark, p.surface)
	case "assistant":
		return p.text
	case "instructions", "tool_output":
		return p.muted
	default:
		return p.faint
	}
}

func agentContextCategoryRows(snapshot agentContextSnapshot) []agentContextCategory {
	kinds := []string{"system", "instructions", "skills", "builtin_tools", "custom_tools", "user", "assistant", "thinking", "tool_calls", "tool_output"}
	rows := make([]agentContextCategory, 0, len(kinds)+1)
	for _, kind := range kinds {
		row := agentContextCategory{kind: kind}
		for _, category := range snapshot.categories {
			if category.kind == kind {
				row = category
				break
			}
		}
		rows = append(rows, row)
	}
	for _, category := range snapshot.categories {
		if category.kind == "unattributed" && category.known && category.tokens > 0 {
			rows = append(rows, category)
			break
		}
	}
	return rows
}

func agentContextPart(tokens, size int) string {
	return agentContextTokenCount(tokens) + " · " + agentContextDetailedPercentage(tokens, size) + "%"
}

func agentContextTokenCount(tokens int) string {
	digits := strconv.Itoa(tokens)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func agentContextDetailedPercentage(tokens, size int) string {
	return agentContextPercentage(acp.UsageState{Used: tokens, Size: size})
}

func (a *app) agentContextPopup(c *ui.Context, s *agentChat, snapshot agentContextSnapshot) {
	if !s.contextDetailsOpen {
		return
	}
	p, m := a.pal, a.pal.metricsOr()
	windowWidth, windowHeight := c.Size()
	width := min(float32(680), max(0, windowWidth-2*m.inset))
	height := max(0, windowHeight-2*m.inset)
	headerHeight, footerHeight := m.control+2*m.inset, m.control+2*m.pad
	ui.DialogBase(c.Key("agent-context-details"), &s.contextDetailsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.28))
		a.floating(c, panel.Column().Width(width).MaxHeight(height).Radius(m.radiusCard+m.pad), true).
			Label(locales.AgentContextDetails()).Clip()
		panel.Children(func() {
			ui.Row(c.Key("title")).FillWidth().MinWidth(0).MinHeight(headerHeight).Shrink(0).Padding(m.inset, 2*m.inset).Gap(m.gap).
				AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AgentContext()).FontSize(m.textTitle + m.gap/2).FontWeight(m.weightMedium).TextColor(p.text).Shrink(0)
				if snapshot.model != "" {
					ui.Text(c, "· "+snapshot.model).FontSize(m.textTitle + m.gap/2).TextColor(p.muted).SingleLine().Grow(1).MinWidth(0).Tooltip(snapshot.model)
				}
			})
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(p.border)
			ui.Scroll(c.Key("context-breakdown")).FillWidth().MinWidth(0).MinHeight(0).
				MaxHeight(max(0, height-headerHeight-footerHeight-2)).Padding(m.inset, 2*m.inset).Gap(m.inset).Children(func() {
				a.agentContextSummary(c, snapshot, width)
				ui.Box(c.Key("segments")).FillWidth().Height(m.gap*2).Shrink(0).Margin(m.gap, 0).Draw(func(pt *ui.Painter, r ui.Rect) {
					a.paintAgentContextSegments(pt, r, snapshot)
				})
				ui.Column(c.Key("categories")).FillWidth().MinWidth(0).Gap(m.gap / 2).Children(func() {
					for _, category := range agentContextCategoryRows(snapshot) {
						label := agentContextCategoryLabel(category.kind)
						detail, tooltip := locales.AgentContextUnreported(), locales.AgentContextUnreported()
						if snapshot.known && category.known && category.tokens >= 0 && snapshot.size > 0 {
							detail = agentContextPart(category.tokens, snapshot.size)
							tooltip = locales.AgentContextUsage(strconv.Itoa(category.tokens), strconv.Itoa(snapshot.size))
						}
						a.agentContextDetailRow(c.Key(category.kind), label, detail, tooltip, agentContextCategoryColor(p, category.kind), false, false)
					}
					free := locales.AgentContextUnreported()
					freeTip := free
					if snapshot.known && snapshot.size > 0 {
						free = agentContextPart(max(0, snapshot.size-snapshot.used), snapshot.size)
						freeTip = locales.AgentContextUsage(strconv.Itoa(max(0, snapshot.size-snapshot.used)), strconv.Itoa(snapshot.size))
					}
					a.agentContextDetailRow(c.Key("free"), locales.AgentContextFreeSpace(), free, freeTip, p.muted, false, true)
				})
				ui.Box(c).FillWidth().Height(1).Shrink(0).Background(p.border)
				ui.Column(c.Key("compaction")).FillWidth().MinWidth(0).Gap(m.gap).Children(func() {
					if status := agentContextCompactionStatusLabel(snapshot.compactionStatus); status != "" {
						a.agentContextDetailRow(c.Key("status"), locales.AgentContextCompactionStatus(), status, status, p.muted, false, false)
						return
					}
					buffer := locales.AgentContextUnreported()
					bufferTip := buffer
					if _, ok := agentContextCompactionFraction(snapshot); ok {
						reserved := snapshot.size - max(0, min(snapshot.compactAt, snapshot.size))
						buffer = agentContextPart(reserved, snapshot.size)
						bufferTip = locales.AgentContextUsage(strconv.Itoa(reserved), strconv.Itoa(snapshot.size)) + "\n" + locales.AgentContextReserveHint()
					} else if snapshot.compactionKnown && !snapshot.compactionEnabled {
						buffer = locales.AgentContextCompactionDisabled()
						bufferTip = buffer
					}
					a.agentContextDetailRow(c.Key("buffer"), locales.AgentContextCompactBuffer(), buffer, bufferTip, p.modifiedFg, true, false)
					if snapshot.compactionKnown && snapshot.compactionEnabled {
						ui.Text(c, locales.AgentContextReserveHint()).FontSize(m.textSmall).TextColor(p.muted).FillWidth().MinWidth(0)
					}
				})
				if snapshot.estimated {
					ui.Text(c, locales.AgentContextEstimated()).FontSize(m.textSmall).TextColor(p.muted).FillWidth().MinWidth(0)
				} else if snapshot.known {
					for _, category := range agentContextCategoryRows(snapshot) {
						if !category.known {
							ui.Text(c, locales.AgentContextBreakdownUnavailable()).FontSize(m.textSmall).TextColor(p.muted).FillWidth().MinWidth(0)
							break
						}
					}
				} else {
					ui.Text(c, locales.AgentContextUnavailable()).FontSize(m.textSmall).TextColor(p.muted).FillWidth().MinWidth(0)
				}
				if snapshot.compactionKnown {
					ui.Text(c, locales.AgentContextCompactionHint()).FontSize(m.textSmall).TextColor(p.muted).FillWidth().MinWidth(0)
				}
			})
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(p.border)
			ui.Row(c.Key("footer")).FillWidth().MinHeight(footerHeight).Shrink(0).Padding(m.pad, 2*m.inset).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("close"), locales.ButtonClose()).Height(m.control).FontSize(m.textBody).Padding(0, m.inset).Radius(m.radiusFull).Clicked() {
					s.contextDetailsOpen = false
				}
			})
		})
	})
}

func (a *app) agentContextSummary(c *ui.Context, snapshot agentContextSnapshot, width float32) {
	p, m := a.pal, a.pal.metricsOr()
	used, summary := "—", locales.AgentContextUnreported()
	remaining := locales.AgentContextCompactionUnavailable()
	if snapshot.known && snapshot.size > 0 {
		used = agentContextTokenCount(snapshot.used)
		summary = locales.AgentContextUsageSummary(agentContextTokenCount(snapshot.size), agentContextPercentage(acp.UsageState{Used: snapshot.used, Size: snapshot.size}))
		if _, ok := agentContextCompactionFraction(snapshot); ok {
			threshold := max(0, min(snapshot.compactAt, snapshot.size))
			remaining = locales.AgentContextBeforeCompaction(agentContextTokenCount(max(0, threshold-snapshot.used)), agentContextDetailedPercentage(threshold, snapshot.size))
		} else if snapshot.compactionKnown && !snapshot.compactionEnabled {
			remaining = locales.AgentContextCompactionDisabled()
		}
	}
	box := ui.Row(c.Key("summary")).FillWidth().MinWidth(0).Gap(m.inset).AlignItems(ui.Center)
	if width < 520 {
		box.Column().AlignItems(ui.Start).Gap(m.gap)
	}
	box.Children(func() {
		usage := ui.Row(c.Key("used")).MinWidth(0).Gap(m.gap).AlignItems(ui.Center)
		if width < 520 {
			usage.Column().AlignItems(ui.Start).Gap(m.gap / 2)
		}
		usage.Children(func() {
			ui.Text(c, used).FontSize(m.textTitle * 2.8).FontFeatures("tnum").FontWeight(m.weightBold).TextColor(p.text).Shrink(0)
			ui.Text(c, summary).FontSize(m.textSmall).FontFeatures("tnum").TextColor(p.muted).MinWidth(0).Shrink(1)
		})
		if width >= 520 {
			ui.Spacer(c).MinWidth(0)
		}
		ui.Text(c, remaining).FontSize(m.textSmall).FontFeatures("tnum").TextColor(p.muted).MinWidth(0).Shrink(1)
	})
}

func (a *app) agentContextDetailRow(c *ui.Context, label, detail, tooltip string, color ui.Color, striped, outlined bool) {
	p, m := a.pal, a.pal.metricsOr()
	ui.Row(c).FillWidth().MinWidth(0).MinHeight(m.row + m.gap).Gap(m.gap * 2).AlignItems(ui.Center).Tooltip(tooltip).Children(func() {
		ui.Box(c).Size(m.gap*2, m.gap*2).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) {
			switch {
			case striped:
				paintAgentContextStripes(pt, r, color, m.gap)
			case outlined:
				pt.Stroke(r, color, 1, 1)
			default:
				pt.Fill(r, color, 1)
			}
		})
		ui.Text(c, label).FontSize(m.textBody).TextColor(p.text).Grow(1).MinWidth(0)
		ui.Text(c, detail).FontSize(m.textSmall).FontFeatures("tnum").TextColor(p.muted).MinWidth(0).Shrink(1)
	})
}

func (a *app) paintAgentContextSegments(pt *ui.Painter, r ui.Rect, snapshot agentContextSnapshot) {
	p, m := a.pal, a.pal.metricsOr()
	pt.Fill(r, p.border, r.H/2)
	if !snapshot.known || snapshot.size <= 0 {
		return
	}
	pt.Clip(r, r.H/2, func() {
		if threshold, ok := agentContextCompactionFraction(snapshot); ok {
			reserve := ui.Rect{X: r.X + r.W*threshold, Y: r.Y, W: r.W * (1 - threshold), H: r.H}
			paintAgentContextStripes(pt, reserve, p.modifiedFg, m.gap)
		}
		used := min(max(snapshot.used, 0), snapshot.size)
		pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(float64(used)/float64(snapshot.size)), H: r.H}, p.faint, 0)
		filled := 0
		for _, category := range snapshot.categories {
			if !category.known || category.tokens <= 0 || filled >= used {
				continue
			}
			tokens := min(category.tokens, used-filled)
			segment := ui.Rect{X: r.X + r.W*float32(float64(filled)/float64(snapshot.size)), Y: r.Y, W: r.W * float32(float64(tokens)/float64(snapshot.size)), H: r.H}
			pt.Fill(segment, agentContextCategoryColor(p, category.kind), 0)
			filled += tokens
		}
	})
	if threshold, ok := agentContextCompactionFraction(snapshot); ok {
		paintAgentContextMarker(pt, r, threshold, p.modifiedFg, m.gap)
	}
}

func paintAgentContextMarker(pt *ui.Painter, r ui.Rect, threshold float32, color ui.Color, gap float32) {
	width := min(float32(1), r.W)
	x := max(r.X, min(r.X+r.W-width, r.X+r.W*threshold-width/2))
	pt.Fill(ui.Rect{X: x, Y: r.Y - gap/2, W: width, H: r.H + gap}, color, 0)
}

func paintAgentContextStripes(pt *ui.Painter, r ui.Rect, color ui.Color, gap float32) {
	pt.Fill(r, color.Alpha(0.16), 0)
	pt.Clip(r, 0, func() {
		var stripes ui.Path
		for x := r.X - r.H; x < r.X+r.W; x += max(gap, 1) {
			stripes.MoveTo(x, r.Y+r.H).LineTo(x+r.H, r.Y)
		}
		pt.StrokePath(&stripes, 1, color.Alpha(0.8))
	})
}
