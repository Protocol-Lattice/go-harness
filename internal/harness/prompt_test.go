package harness

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptUsesConsistentShellRunGuidance(t *testing.T) {
	prompt := BuildSystemPrompt(nil, ".")

	for _, expected := range []string{
		"Prefer shell.run with argv when available",
		"shell.run accepts argv for simple commands",
		"Use codemode.run_code for multi-tool workflows",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("system prompt missing %q:\n%s", expected, prompt)
		}
	}

	for _, forbidden := range []string{
		`shell.run expects {"command":"..."} as a single string`,
		"Never pass shell commands as arrays",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("system prompt contains stale shell guidance %q:\n%s", forbidden, prompt)
		}
	}
}
