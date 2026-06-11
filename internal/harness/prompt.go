package harness

import (
	"strings"
)

func BuildSystemPrompt(skills []Skill, workspace string) string {
	var b strings.Builder

	b.WriteString(`You are an agentic Go TUI coding assistant.

Operating rules:
- Prefer small, testable changes.
- Use loaded skills when relevant.
- Use UTCP tools for filesystem, shell, git, and project inspection.
- Use CodeMode only when tool chaining is useful.
- Before destructive operations, ask for approval.
- Never claim a file was changed unless a tool confirms it.
- For Go projects, run gofmt and relevant go test commands when possible.
- Keep responses concise, practical, and grounded in observed files.

Workspace root:
`)
	b.WriteString(workspace)
	b.WriteString("\n\n")

	if len(skills) == 0 {
		b.WriteString("Loaded skills: none.\n")
		return b.String()
	}

	b.WriteString("Loaded skills:\n")
	for _, s := range skills {
		b.WriteString("\n--- skill: ")
		b.WriteString(s.Name)
		b.WriteString(" ---\n")
		if s.Description != "" {
			b.WriteString("Description: ")
			b.WriteString(s.Description)
			b.WriteString("\n")
		}
		b.WriteString(s.Body)
		b.WriteString("\n")
	}

	return b.String()
}
