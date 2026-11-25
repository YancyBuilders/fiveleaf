package main

import (
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/internal/settings"
)

// This CLI acknowledges startup through a file, then emits no output. A frame
// cannot accidentally discover a settings change because the process replied.
func TestAgentConfigurationCLIProcess(t *testing.T) {
	if os.Getenv("FIVELEAF_AGENT_CONFIGURATION_TEST") == "" {
		return
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	kind := "direct"
	if strings.Contains(string(prompt), "configuration-room") {
		kind = "room"
	}
	if err := os.WriteFile("configuration-"+kind+".ready", []byte("ready"), 0o600); err != nil {
		os.Exit(2)
	}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("configuration.release"); err == nil {
			os.Exit(0)
		}
		time.Sleep(2 * time.Millisecond)
	}
	os.Exit(3)
}

func newAgentConfigurationProjects(t *testing.T) (*app, *app) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := settings.Default()
	s.AgentServers = map[string]settings.AgentServer{
		"quiet": {
			Name: "Quiet", Protocol: "cli", Command: executable,
			Args: []string{"-test.run=^TestAgentConfigurationCLIProcess$"}, TimeoutSeconds: 30,
			Env: map[string]string{"FIVELEAF_AGENT_CONFIGURATION_TEST": "1", "GORACE": "atexit_sleep_ms=0"},
		},
		"idle": {Name: "Idle", Protocol: "cli", Command: executable},
	}
	var projects []*app
	for range 2 {
		root := t.TempDir()
		window := s.NewWindow()
		window.Folder = root
		a := newApp(&fakeHost{folder: root}, s, window)
		// Agent lifetimes do not need a native view. In particular, neither
		// project's pending callbacks are drained during these tests.
		a.root = root
		projects = append(projects, a)
		t.Cleanup(a.closeAllAgents)
	}
	for _, a := range projects {
		a.host.(*fakeHost).projects = projects
	}
	return projects[0], projects[1]
}

func startQuietConfigurationChat(t *testing.T, a *app) *agentChat {
	t.Helper()
	a.selectAgent("quiet")
	s := a.agent
	s.draft = "configuration-direct"
	a.sendAgent()
	if !s.busy || s.ctx == nil || s.connected == nil {
		t.Fatal("quiet CLI did not start")
	}
	// Keep the running chat hidden behind another selected conversation too.
	a.selectAgent("idle")
	a.agent.draft = "keep selected draft"
	return s
}

func waitForConfigurationCLI(t *testing.T, a *app, kind string) {
	t.Helper()
	path := filepath.Join(a.root, "configuration-"+kind+".ready")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("silent %s CLI did not acknowledge startup", kind)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func assertConfigurationChatStopped(t *testing.T, a *app, s *agentChat, ctx context.Context, generation uint64) {
	t.Helper()
	if ctx.Err() != context.Canceled || s.busy || s.connecting || s.connected != nil || s.generation <= generation {
		t.Fatalf("background chat still active before any frame: context=%v busy=%v connecting=%v connected=%v generation=%d (was %d)",
			ctx.Err(), s.busy, s.connecting, s.connected != nil, s.generation, generation)
	}
	if s.err == "" {
		t.Error("configuration cancellation was not explained")
	}
	if a.agent == s || a.agent.provider != "idle" || a.agent.draft != "keep selected draft" {
		t.Error("cancelling hidden chat changed the selected conversation")
	}
	if len(s.messages) != 1 || s.messages[0].role != "user" {
		t.Error("silent CLI produced a reply before it was cancelled")
	}
}

func TestAgentConfigurationDisableStopsOtherProjectBeforeFrame(t *testing.T) {
	front, other := newAgentConfigurationProjects(t)
	frontChat := startQuietConfigurationChat(t, front)
	otherChat := startQuietConfigurationChat(t, other)
	meeting := testAgentMeeting(t, other)
	if err := meeting.room.Send("configuration-room", "quiet"); err != nil {
		t.Fatal(err)
	}
	waitForConfigurationCLI(t, front, "direct")
	waitForConfigurationCLI(t, other, "direct")
	waitForConfigurationCLI(t, other, "room")
	frontCtx, otherCtx := frontChat.ctx, otherChat.ctx
	frontGeneration, otherGeneration := frontChat.generation, otherChat.generation
	if !meeting.room.Snapshot().Busy {
		t.Fatal("quiet roundtable was not running")
	}

	front.setAgentEnabled("quiet", false)

	// These assertions must precede drain or a UI frame: checkAgentConfig in
	// drain would otherwise hide a missing notification to the other project.
	assertConfigurationChatStopped(t, front, frontChat, frontCtx, frontGeneration)
	assertConfigurationChatStopped(t, other, otherChat, otherCtx, otherGeneration)
	if !front.settings.AgentServers["quiet"].Disabled || other.settings != front.settings {
		t.Fatal("disable did not update the shared configuration")
	}
	if snap := meeting.room.Snapshot(); snap.Busy || snap.ActiveAgent != "" || len(snap.Messages) != 1 {
		t.Fatalf("other project's roundtable continued before a frame: %+v", snap)
	}
}

func TestAgentConfigurationReloadStopsOtherProjectBeforeFrame(t *testing.T) {
	for _, change := range []string{"edit", "remove"} {
		t.Run(change, func(t *testing.T) {
			front, other := newAgentConfigurationProjects(t)
			chat := startQuietConfigurationChat(t, other)
			meeting := testAgentMeeting(t, other)
			if err := meeting.room.Send("configuration-room", "quiet"); err != nil {
				t.Fatal(err)
			}
			waitForConfigurationCLI(t, other, "direct")
			waitForConfigurationCLI(t, other, "room")
			ctx, generation := chat.ctx, chat.generation
			m := &fiveleaf{s: front.settings}
			m.wins = []*desktop{{m: m, a: front}, {m: m, a: other}}
			prefs := m.s.Prefs
			prefs.AgentServers = maps.Clone(prefs.AgentServers)
			if change == "remove" {
				delete(prefs.AgentServers, "quiet")
			} else {
				server := prefs.AgentServers["quiet"]
				server.TimeoutSeconds = 60
				prefs.AgentServers["quiet"] = server
			}

			m.reload(prefs)

			assertConfigurationChatStopped(t, other, chat, ctx, generation)
			if snap := meeting.room.Snapshot(); snap.Busy || snap.ActiveAgent != "" || len(snap.Messages) != 1 {
				t.Fatalf("configuration %s left other project's roundtable running before a frame: %+v", change, snap)
			}
		})
	}
}
