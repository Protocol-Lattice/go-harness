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
	readLine    func() (string, bool)
	reader      *bufio.Reader
}

func (g *ApprovalGate) Approve(reason string) bool {
	if g == nil {
		return false
	}
	if g.AutoApprove {
		return true
	}

	if g.readLine == nil && g.In == nil {
		return false
	}

	out := g.Out
	if out == nil {
		out = io.Discard
	}

	fmt.Fprintf(out, "\nApproval required: %s\nApprove? [y/N]: ", reason)

	answer, ok := g.readAnswer()
	if !ok {
		return false
	}

	normalized := strings.ToLower(strings.TrimSpace(answer))
	return normalized == "y" || normalized == "yes"
}

func (g *ApprovalGate) readAnswer() (string, bool) {
	if g.readLine != nil {
		return g.readLine()
	}

	if g.reader == nil {
		g.reader = bufio.NewReader(g.In)
	}
	answer, err := g.reader.ReadString('\n')
	if err != nil && answer == "" {
		return "", false
	}
	return answer, true
}
