package main

import (
	"fmt"
	"strconv"
	"strings"

	"fiveleaf/internal/acp"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (a *app) refreshAgentUsage(s *agentChat) {
	if s.client == nil || s.sessionID == "" {
		s.contextUsage = nil
		return
	}
	usage, ok := s.client.UsageState(s.sessionID)
	if !ok {
		return
	}
	if usage.Used == 0 && !s.sessionPrompted && s.contextUsage == nil {
		return
	}
	if s.usesCodexThread() {
		if tokens, found := s.client.PromptUsage(s.sessionID); found {
			usage = agentContextOccupancy(true, tokens, usage)
		}
	}
	if s.usesPiThread() && s.connected != nil {
		if size, found := piContextWindow(s.connected.config, a.root, s.model(), usage.Size); found {
			usage.Size = size
		}
	}
	s.contextUsage = &usage
}

func agentContextOccupancy(codex bool, prompt acp.PromptUsage, usage acp.UsageState) acp.UsageState {
	if codex && prompt.Input+prompt.CachedInput > 0 {
		usage.Used = prompt.Input + prompt.CachedInput
	}
	return usage
}

func (s *agentChat) acceptContextUsage(usage acp.UsageState) {
	if usage.Used < 0 || usage.Size <= 0 {
		return
	}
	s.contextUsage = &usage
}

func agentTokenCount(value int) string {
	if value < 1000 {
		return strconv.Itoa(value)
	}
	unit, scale := "k", float64(1000)
	if value >= 1000000 {
		unit, scale = "M", 1000000
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(value)/scale), ".0") + unit
}

func agentContextPercentage(usage acp.UsageState) string {
	percent := 100 * float64(usage.Used) / float64(usage.Size)
	if percent > 0 && percent < 0.05 {
		return "<0.1"
	}
	return strings.TrimSuffix(strconv.FormatFloat(percent, 'f', 1, 64), ".0")
}

func (a *app) agentContextUsage(c *ui.Context, s *agentChat) ui.Element {
	p, m := a.pal, a.pal.metricsOr()
	snapshot := a.contextSnapshot(s)
	capacity, percentage, tooltip := "—", "", locales.AgentContextUnavailable()
	color, fraction := p.muted, float32(0)
	if snapshot.known && snapshot.used >= 0 && snapshot.size > 0 {
		ratio := float64(snapshot.used) / float64(snapshot.size)
		capacity = agentTokenCount(snapshot.size)
		percentage = agentContextPercentage(acp.UsageState{Used: snapshot.used, Size: snapshot.size}) + "%"
		tooltip = locales.AgentContextUsage(strconv.Itoa(snapshot.used), strconv.Itoa(snapshot.size)) + "\n" + locales.AgentContextUsageHint()
		color, fraction = p.accent, float32(max(0, min(ratio, 1)))
		switch {
		case ratio >= 0.95:
			color = p.dangerFg
		case ratio >= 0.85:
			color = p.modifiedFg
		}
	}
	if s.usesCodexThread() {
		tooltip += "\n" + locales.AgentContextCodexUsageHint()
	}
	if snapshot.compactionKnown {
		tooltip += "\n" + locales.AgentContextCompactionHint()
	}
	if status := agentContextCompactionStatusLabel(snapshot.compactionStatus); status != "" {
		tooltip += "\n" + status
	}
	inset := m.radiusCard + m.pad + m.gap/2
	button := ui.ButtonBase(c.Key("agent-context-usage")).Column().FillWidth().Height(m.row).MinWidth(0).Shrink(0).
		Justify(ui.End).Gap(m.gap / 2).Label(locales.AgentContext()).Tooltip(tooltip).Cursor(ui.CursorPointer)
	button.Children(func() {
		ui.Row(c.Key("capacity")).FillWidth().MinWidth(0).Padding(0, inset).Justify(ui.End).Children(func() {
			if percentage != "" {
				ui.Text(c.Key("percentage"), percentage).FontSize(m.textSmall).FontFeatures("tnum").TextColor(p.muted).SingleLine().Shrink(0)
			}
			ui.Spacer(c).MinWidth(0)
			ui.Text(c, locales.AgentContextCapacity(capacity)).FontSize(m.textSmall).FontFeatures("tnum").TextColor(p.muted).SingleLine().Shrink(0)
		})
		ui.Box(c.Key("track")).FillWidth().Height(2).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) {
			padding := min(inset, r.W/2)
			r.X += padding
			r.W -= 2 * padding
			pt.Fill(r, p.border, 0)
			if fraction > 0 {
				pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * fraction, H: r.H}, color, 0)
			}
			if threshold, ok := agentContextCompactionFraction(snapshot); ok {
				paintAgentContextMarker(pt, r, threshold, p.modifiedFg, m.gap)
			}
		})
	})
	if button.Clicked() {
		s.contextDetailsOpen = !s.contextDetailsOpen
	}
	a.agentContextPopup(c, s, snapshot)
	return button
}
