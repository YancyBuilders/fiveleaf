package main

import (
	"strings"
	"testing"
)

func TestRoundtableACPRecordsOnlyPromptReply(t *testing.T) {
	for _, scenario := range []string{"welcome-on-open", "late-startup"} {
		t.Run(scenario, func(t *testing.T) {
			hs, _ := agentRoleHarness(t, scenario)
			meeting := testAgentMeeting(t, hs.a)
			if err := meeting.room.Discuss("Timing", "Use the frozen estimate", "backend"); err != nil {
				t.Fatal(err)
			}
			hs.untilLong("scoped ACP roundtable reply", func() bool { return !meeting.room.Snapshot().Busy })
			snapshot := meeting.room.Snapshot()
			if snapshot.Error != "" || len(snapshot.Messages) != 2 || strings.Contains(snapshot.Messages[1].Text, "Adapter startup details marker") || !strings.Contains(snapshot.Messages[1].Text, "Use the frozen estimate") {
				t.Fatal(snapshot)
			}
		})
	}
}

func TestRoundtableACPSeparatesPreToolUpdateFromAnswer(t *testing.T) {
	hs, _ := agentRoleHarness(t, "roundtable-tool-preamble")
	meeting := testAgentMeeting(t, hs.a)
	if err := meeting.room.Discuss("Image fallback", "Read the requirement", "backend"); err != nil {
		t.Fatal(err)
	}
	hs.untilLong("tool and final reply", func() bool { return !meeting.room.Snapshot().Busy })
	snap := meeting.room.Snapshot()
	if snap.Error != "" || len(snap.Messages) != 2 {
		t.Fatal(snap)
	}
	msg := snap.Messages[1]
	if msg.Text != "120秒" || len(msg.Task.Tools) != 1 || msg.Task.Tools[0].Note != "Looking up the fallback" {
		t.Fatal("public answer and tool step were mixed", msg)
	}
}
