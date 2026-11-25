package roundtable

import "testing"

func TestLegacyPiAnswer(t *testing.T) {
	banner := "pi v1.5.2\n---\n\n## Skills\n- /example/.agents/skills/review/SKILL.md\n\n## Extensions\n- /example/.pi/agent/extensions/topic.ts\n"
	for input, want := range map[string]string{
		banner + "- /repo/main.go\n- /repo/plan.md": "- /repo/main.go\n- /repo/plan.md",
		banner + "@Reviewer review the plan":        "@Reviewer review the plan",
		banner:                                      "", "## Skills\n- Keep this list": "## Skills\n- Keep this list",
		"pi v1.5.2\n---\n\n## Skills\n- This is my answer": "pi v1.5.2\n---\n\n## Skills\n- This is my answer",
		"pi v1.5.2 was released":                           "pi v1.5.2 was released",
	} {
		if got := legacyPiAnswer(input); got != want {
			t.Errorf("clean %q = %q, want %q", input, got, want)
		}
	}
}
