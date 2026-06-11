package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilesystemWriteAcceptsUTCPCLICallConvention(t *testing.T) {
	root := t.TempDir()

	out := runFilesystemHelper(t, root, "call", "filesystem", "filesystem.write").
		withStdin(`{"path":"nested/hello.txt","content":"hello world"}`).
		run()

	var result Result
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, out)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %+v", result)
	}

	got, err := os.ReadFile(filepath.Join(root, "nested", "hello.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("written content = %q, want %q", got, "hello world")
	}
}

type filesystemHelper struct {
	t     *testing.T
	root  string
	args  []string
	stdin string
}

func runFilesystemHelper(t *testing.T, root string, args ...string) filesystemHelper {
	t.Helper()
	return filesystemHelper{t: t, root: root, args: args}
}

func (h filesystemHelper) withStdin(stdin string) filesystemHelper {
	h.stdin = stdin
	return h
}

func (h filesystemHelper) run() []byte {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := append([]string{"-test.run=TestFilesystemHelperProcess", "--", "--root", h.root}, h.args...)
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_WANT_FILESYSTEM_HELPER_PROCESS=1")
	cmd.Stdin = strings.NewReader(h.stdin)

	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("helper failed: %v\n%s", err, out)
	}
	return out
}

func TestFilesystemHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_FILESYSTEM_HELPER_PROCESS") != "1" {
		return
	}

	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"filesystem"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}

	os.Exit(2)
}
