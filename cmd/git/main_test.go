package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestGitStatusAcceptsUTCPCLICallConvention(t *testing.T) {
	root := t.TempDir()
	initCmd := exec.Command("git", "init", root)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	out := runGitHelper(t, root, "call", "git", "git.status").
		withStdin(`{"short":true}`).
		run()

	var result Result
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, out)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %+v", result)
	}
	if !strings.Contains(result.Command, "git status") {
		t.Fatalf("command = %q, want git status", result.Command)
	}
}

type gitHelper struct {
	t     *testing.T
	root  string
	args  []string
	stdin string
}

func runGitHelper(t *testing.T, root string, args ...string) gitHelper {
	t.Helper()
	return gitHelper{t: t, root: root, args: args}
}

func (h gitHelper) withStdin(stdin string) gitHelper {
	h.stdin = stdin
	return h
}

func (h gitHelper) run() []byte {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := append([]string{"-test.run=TestGitHelperProcess", "--", "--root", h.root}, h.args...)
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_WANT_GIT_HELPER_PROCESS=1")
	cmd.Stdin = strings.NewReader(h.stdin)

	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("helper failed: %v\n%s", err, out)
	}
	return out
}

func TestGitHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_GIT_HELPER_PROCESS") != "1" {
		return
	}

	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"git"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}

	os.Exit(2)
}
