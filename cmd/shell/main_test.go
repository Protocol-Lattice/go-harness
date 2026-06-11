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

func TestShellRunAcceptsUTCPCLICallConventionWithArgv(t *testing.T) {
	out := runShellHelper(t, t.TempDir(), "call", "shell", "shell.run").
		withStdin(`{"argv":["/bin/echo","hello from argv"]}`).
		run()

	var result Result
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, out)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %+v", result)
	}
	if strings.TrimSpace(result.Stdout) != "hello from argv" {
		t.Fatalf("stdout = %q, want %q", result.Stdout, "hello from argv")
	}
}

type shellHelper struct {
	t     *testing.T
	root  string
	args  []string
	stdin string
}

func runShellHelper(t *testing.T, root string, args ...string) shellHelper {
	t.Helper()
	return shellHelper{t: t, root: root, args: args}
}

func (h shellHelper) withStdin(stdin string) shellHelper {
	h.stdin = stdin
	return h
}

func (h shellHelper) run() []byte {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := append([]string{"-test.run=TestShellHelperProcess", "--", "--root", h.root}, h.args...)
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_WANT_SHELL_HELPER_PROCESS=1")
	cmd.Stdin = strings.NewReader(h.stdin)

	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("helper failed: %v\n%s", err, out)
	}
	return out
}

func TestShellHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SHELL_HELPER_PROCESS") != "1" {
		return
	}

	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"shell"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}

	os.Exit(2)
}
