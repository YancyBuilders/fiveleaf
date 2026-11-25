package main

import (
	"bytes"
	"image"
	"image/draw"
	"testing"

	"fiveleaf/internal/agents"
	"fiveleaf/internal/settings"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func TestChatAgentMarksOnTabAndComposer(t *testing.T) {
	hs := newHarness(t, false)
	providers := []struct{ id, name string }{
		{"custom-pi", "Pi"}, {"omp", "Oh My Pi"}, {"agent-opencode.acp", "OpenCode"},
		{"agent-claude.acp", "Claude Code"}, {"agent-codex.acp", "Codex"}, {"grok-build", "Grok Build"},
		{"agent-gemini.acp", "Gemini CLI"}, {"agent-qwen.acp", "Qwen Code"}, {"agent-kimi.acp", "Kimi Code"},
	}
	hs.a.settings.AgentServers = map[string]settings.AgentServer{}
	for _, provider := range providers {
		hs.a.settings.AgentServers[provider.id] = settings.AgentServer{Name: provider.name, Protocol: "cli", Command: "unused"}
	}
	hs.a.openAgentPage()
	sizeChatWorkspace(hs, 340)
	seen := map[string]map[string]string{}
	for _, provider := range providers {
		hs.a.selectAgent(provider.id)
		hs.settle()
		for _, label := range []string{locales.AgentSelectionAutomatic(), locales.AgentChoose()} {
			slot, found := hs.tt.Find(label)
			if !found || slot.W > 24 {
				t.Fatalf("%s: missing compact icon for %s: %v", provider.name, label, slot)
			}
			pixels := chatMarkPixels(hs.tt.Image(), slot)
			if bytes.Equal([]byte(pixels), bytes.Repeat([]byte(pixels[:4]), len(pixels)/4)) {
				t.Errorf("%s: %s has an empty icon", label, provider.name)
			}
			if seen[label] == nil {
				seen[label] = map[string]string{}
			}
			if previous := seen[label][pixels]; previous != "" {
				t.Errorf("%s: %s and %s render the same glyph", label, previous, provider.name)
			}
			seen[label][pixels] = provider.name
		}
		hs.shot("agent-mark-" + provider.id)
	}
}

func TestChatMarkIdentity(t *testing.T) {
	pal := newHarness(t, false).a.pal
	for _, test := range []struct {
		id, name, command, mark string
		args                    []string
	}{
		{"pi", "Slider Demo", "node", "pi-acp", nil},
		{"my-wrapper", "Oh My Pi", "wrapper", "omp", nil},
		{"custom-backend", "Personal", "/local/bin/omp", "omp", nil},
		{"local", "Reviewer", "npx", "pi-acp", []string{"-y", "pi-acp@0.0.4"}},
		{"local", "Reviewer", "npx", "grok-build", []string{"-y", "@xai-official/grok@1.0.89", "agent", "stdio"}},
		{"local", "Reviewer", "node", "codex-acp", []string{"/node_modules/@agentclientprotocol/codex-acp/dist/index.js"}},
		{"local", "Personal", "opencode.exe", "opencode", []string{"acp"}},
		{"custom-grok-build", "Team", "wrapper", "grok-build", nil},
	} {
		provider := agentProvider{id: test.id, name: test.name, config: settings.AgentServer{Command: test.command, Args: test.args}}
		if got := provider.chatMark(pal); got != chatMarks[test.mark] {
			t.Errorf("%s (%s %v) did not resolve to %q", test.id, test.command, test.args, test.mark)
		}
	}
	for _, provider := range []agentProvider{
		{},
		{id: "compass", name: "Pioneer", config: settings.AgentServer{Command: "npx", Args: []string{"unknown", "--model", "pi"}}},
	} {
		if provider.chatMark(pal) != iconAgent {
			t.Errorf("chat with provider %q should use the robot icon", provider.id)
		}
	}
	qoder, _ := agentIcon(pal, agents.Qoder)
	if got := (agentProvider{id: "my-tool", name: "Qoder"}).chatMark(pal); got != qoder || got == iconAgent {
		t.Fatal("a registered agent mark was ignored")
	}
}

func TestAgentMenuIconPixels(t *testing.T) {
	for _, dark := range []bool{false, true} {
		pal := newHarness(t, dark).a.pal
		seen := map[string]string{}
		for _, name := range []string{"Pi", "OMP", "OpenCode", "Claude Code", "Codex", "Grok Build", "Gemini CLI", "Qwen Code", "Kimi Code", "Qoder", "个人助手"} {
			mark := (agentProvider{id: name, name: name}).chatMark(pal)
			img := ui.Render(func(c *ui.Context) {
				ui.Box(c).Fill().Center().Background(pal.panel).Children(func() {
					ui.Icon(c, mark).FontSize(16).TextColor(pal.text)
				})
			}, 16, 16, 2)
			if img.Rect != image.Rect(0, 0, 32, 32) {
				t.Fatalf("%s: wrong Retina menu icon size: %v", name, img.Rect)
			}
			var mask []byte
			var clear, ink int
			for index := 0; index < len(img.Pix); index += 4 {
				pixel := ui.RGB(img.Pix[index], img.Pix[index+1], img.Pix[index+2])
				mask = append(mask, pixel.R)
				if pixel == pal.panel {
					clear++
				} else if contrast(pixel, pal.panel) >= 2 {
					ink++
				}
			}
			if clear < 50 || ink < 25 || seen[string(mask)] != "" {
				t.Fatalf("%s: invalid or repeated icon mask (dark=%v): clear=%d ink=%d duplicate=%q", name, dark, clear, ink, seen[string(mask)])
			}
			seen[string(mask)] = name
		}
	}
}

func TestChatMarkFollowsRoleInSplit(t *testing.T) {
	hs := newHarness(t, false)
	hs.a.settings.AgentServers = map[string]settings.AgentServer{
		"pi": {Name: "Pi", Protocol: "cli", Command: "pi"}, "omp": {Name: "OMP", Protocol: "cli", Command: "omp"},
	}
	hs.a.settings.AgentRoles["Reviewer"] = settings.AgentRole{Agent: "pi"}
	hs.a.openAgentPage()
	hs.a.selectAgent("pi")
	sizeChatWorkspace(hs, 680)
	hs.settle()
	left := hs.a.agent
	hs.a.splitRight()
	hs.settle()
	mark := func() string {
		t.Helper()
		bounds, ok := hs.tt.Find(locales.AgentSelectionAutomatic())
		if !ok {
			t.Fatal("chat badge is missing")
		}
		return chatMarkPixels(hs.tt.Image(), bounds)
	}
	hs.a.selectAgent("pi")
	hs.settle()
	piMark := mark()
	hs.a.selectAgentRole("Reviewer")
	hs.settle()
	if mark() != piMark {
		t.Fatal("a role renamed its provider's mark")
	}
	hs.a.selectAgent("omp")
	hs.settle()
	if mark() == piMark {
		t.Fatal("switching the receiving pane to OMP kept Pi's mark")
	}
	input := left.input.Bounds(hs.a.review.ctx)
	hs.tt.ClickAt(input.X+input.W/2, input.Y+input.H/2)
	hs.settle()
	if hs.a.agent != left || mark() != piMark {
		t.Fatal("focusing the other split did not restore its own mark")
	}
}

func chatMarkPixels(frame *image.RGBA, slot ui.Rect) string {
	center := image.Pt(int(slot.X+slot.W/2), int(slot.Y+slot.H/2))
	crop := image.NewRGBA(image.Rect(0, 0, 14, 14))
	draw.Draw(crop, crop.Bounds(), frame, center.Sub(image.Pt(7, 7)), draw.Src)
	return string(crop.Pix)
}
