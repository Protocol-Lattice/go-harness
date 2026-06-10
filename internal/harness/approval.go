package harness

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type ApprovalGate struct {
	AutoApprove bool
	In          io.Reader
	Out         io.Writer
}

func (g ApprovalGate) Approve(reason string) bool {
	if g.AutoApprove {
		return true
	}

	in := g.In
	if in == nil {
		return false
	}

	out := g.Out
	if out == nil {
		out = io.Discard
	}

	fmt.Fprintf(out, "\nApproval required: %s\nApprove? [y/N]: ", reason)

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}
