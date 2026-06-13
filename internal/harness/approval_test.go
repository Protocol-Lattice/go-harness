package harness

import (
	"bufio"
	"strings"
	"testing"
)

func TestApprovalGateReadsSequentialAnswersFromSameReader(t *testing.T) {
	var out strings.Builder
	gate := &ApprovalGate{
		In:  strings.NewReader("y\ny\n"),
		Out: &out,
	}

	if !gate.Approve("tool filesystem.mkdir args={}") {
		t.Fatal("first approval was denied, want approved")
	}
	if !gate.Approve("tool filesystem.write args={}") {
		t.Fatal("second approval was denied, want approved")
	}
}

func TestApprovalGateCanShareLoopScanner(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("task\ny\n/exit\n"))
	if !scanner.Scan() {
		t.Fatal("scan task line")
	}
	if scanner.Text() != "task" {
		t.Fatalf("first line = %q, want task", scanner.Text())
	}

	var out strings.Builder
	gate := &ApprovalGate{Out: &out}
	gate.readLine = func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	if !gate.Approve("tool filesystem.mkdir args={}") {
		t.Fatal("approval was denied, want approved")
	}
	if !scanner.Scan() {
		t.Fatal("scan line after approval")
	}
	if scanner.Text() != "/exit" {
		t.Fatalf("line after approval = %q, want /exit", scanner.Text())
	}
}

func TestRuntimeBindsApprovalToScanner(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("task\ny\n/exit\n"))
	if !scanner.Scan() {
		t.Fatal("scan task line")
	}

	var out strings.Builder
	rt := &Runtime{
		gate: &ApprovalGate{Out: &out},
	}

	restore := rt.bindApprovalScanner(scanner)
	defer restore()

	if !rt.gate.Approve("tool filesystem.mkdir args={}") {
		t.Fatal("approval was denied, want approved")
	}
	if !scanner.Scan() {
		t.Fatal("scan line after approval")
	}
	if scanner.Text() != "/exit" {
		t.Fatalf("line after approval = %q, want /exit", scanner.Text())
	}
}
