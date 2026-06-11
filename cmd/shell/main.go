package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second
const maxOutputBytes = 512 * 1024

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Inputs      map[string]any `json:"inputs,omitempty"`
}

type RunInput struct {
	Command        string            `json:"command"`
	Argv           []string          `json:"argv,omitempty"`
	Args           []string          `json:"args,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`

	// Dangerous escape hatch. Keep false by default.
	// When true, command is executed through the system shell.
	// Requires HARNESS_ALLOW_SHELL=1.
	UseShell bool `json:"use_shell,omitempty"`
}

type Result struct {
	OK       bool   `json:"ok"`
	Tool     string `json:"tool"`
	Command  string `json:"command"`
	Dir      string `json:"dir"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func main() {
	root, args, err := parseArgs(os.Args[1:])
	if err != nil {
		writeError("", "", "", err, 2)
		os.Exit(2)
	}

	if len(args) == 0 {
		writeError("", "", "", errors.New("missing command"), 2)
		os.Exit(2)
	}
	args, err = normalizeUTCPCallArgs(args, os.Stdin)
	if err != nil {
		writeError("", "", "", err, 2)
		os.Exit(2)
	}

	if args[0] == "list-tools" {
		writeJSON(tools())
		return
	}

	if args[0] != "shell.run" {
		writeError(args[0], "", "", fmt.Errorf("unknown tool %q", args[0]), 2)
		os.Exit(2)
	}

	if len(args) < 2 {
		writeError("shell.run", "", "", errors.New("missing json input argument"), 2)
		os.Exit(2)
	}

	var in RunInput
	mustDecode([]byte(args[1]), &in)
	in = normalizeRunInput(in)

	if err := validateInput(in); err != nil {
		writeError("shell.run", "", "", err, 2)
		os.Exit(2)
	}

	runShell(root, in)
}

func parseArgs(args []string) (string, []string, error) {
	fs := flag.NewFlagSet("shell-provider", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	root := fs.String("root", ".", "workspace root")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return "", nil, err
	}

	return absRoot, fs.Args(), nil
}

func normalizeUTCPCallArgs(args []string, stdin io.Reader) ([]string, error) {
	if len(args) == 0 || args[0] != "call" {
		return args, nil
	}
	if len(args) < 3 {
		return nil, errors.New("usage: call <provider> <tool>")
	}

	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin json input: %w", err)
	}

	return []string{args[2], strings.TrimSpace(string(raw))}, nil
}

func tools() []Tool {
	return []Tool{
		{
			Name:        "shell.run",
			Description: "Run a non-interactive command inside the workspace root. Uses argv mode by default; shell mode requires HARNESS_ALLOW_SHELL=1.",
			Inputs: map[string]any{
				"argv":            "optional full argv list, e.g. [\"go\", \"test\", \"./...\"]",
				"command":         "required executable name, e.g. go",
				"args":            "optional argv list, e.g. [\"test\", \"./...\"]",
				"cwd":             "optional working directory relative to root",
				"env":             "optional environment map",
				"timeout_seconds": "optional timeout, max 300",
				"use_shell":       "optional bool; requires HARNESS_ALLOW_SHELL=1",
			},
		},
	}
}

func normalizeRunInput(in RunInput) RunInput {
	if strings.TrimSpace(in.Command) != "" || len(in.Argv) == 0 {
		return in
	}

	in.Command = in.Argv[0]
	if len(in.Argv) > 1 {
		in.Args = append([]string{}, in.Argv[1:]...)
	}
	return in
}

func validateInput(in RunInput) error {
	if strings.TrimSpace(in.Command) == "" {
		return errors.New("command is required")
	}

	if strings.ContainsRune(in.Command, '\x00') {
		return errors.New("command contains NUL byte")
	}

	for _, arg := range in.Args {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("argument contains NUL byte")
		}
	}

	if in.UseShell && os.Getenv("HARNESS_ALLOW_SHELL") != "1" {
		return errors.New("shell mode is disabled; set HARNESS_ALLOW_SHELL=1 to allow it")
	}

	if isBlockedCommand(in.Command) {
		return fmt.Errorf("blocked command %q", in.Command)
	}

	return nil
}

func runShell(root string, in RunInput) {
	dir, err := safeDir(root, in.CWD)
	if err != nil {
		writeError("shell.run", printableCommand(in), "", err, 2)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout(in.TimeoutSeconds))
	defer cancel()

	cmd := buildCommand(ctx, in)
	cmd.Dir = dir
	cmd.Env = cleanEnv(in.Env)

	var stdout limitedBuffer
	var stderr limitedBuffer
	stdout.limit = maxOutputBytes
	stderr.limit = maxOutputBytes

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	result := Result{
		OK:      err == nil,
		Tool:    "shell.run",
		Command: printableCommand(in),
		Dir:     dir,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
	}

	if stdout.truncated {
		result.Stdout += "\n[stdout truncated]\n"
	}
	if stderr.truncated {
		result.Stderr += "\n[stderr truncated]\n"
	}

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = exitCode(err)
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.Error = "command timed out"
		result.ExitCode = 124
	}

	writeJSON(result)

	if !result.OK {
		os.Exit(1)
	}
}

func buildCommand(ctx context.Context, in RunInput) *exec.Cmd {
	if in.UseShell {
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd", "/C", in.Command)
		}
		return exec.CommandContext(ctx, "sh", "-c", in.Command)
	}

	return exec.CommandContext(ctx, in.Command, in.Args...)
}

func cleanEnv(extra map[string]string) []string {
	env := os.Environ()

	// Remove high-risk dynamic loader env vars.
	blockedPrefixes := []string{
		"LD_PRELOAD=",
		"LD_LIBRARY_PATH=",
		"DYLD_INSERT_LIBRARIES=",
		"DYLD_LIBRARY_PATH=",
	}

	filtered := env[:0]
	for _, item := range env {
		blocked := false
		for _, prefix := range blockedPrefixes {
			if strings.HasPrefix(item, prefix) {
				blocked = true
				break
			}
		}
		if !blocked {
			filtered = append(filtered, item)
		}
	}

	for k, v := range extra {
		if !validEnvKey(k) {
			continue
		}
		filtered = append(filtered, k+"="+v)
	}

	return filtered
}

func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	if strings.ContainsAny(k, "=\x00") {
		return false
	}
	return true
}

func isBlockedCommand(command string) bool {
	base := filepath.Base(command)

	blocked := map[string]bool{
		"sudo":     true,
		"su":       true,
		"ssh":      true,
		"scp":      true,
		"sftp":     true,
		"curl":     true,
		"wget":     true,
		"nc":       true,
		"netcat":   true,
		"mkfs":     true,
		"mount":    true,
		"umount":   true,
		"shutdown": true,
		"reboot":   true,
	}

	return blocked[base]
}

func safeDir(root, cwd string) (string, error) {
	if cwd == "" {
		cwd = "."
	}

	target := cwd
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, cwd)
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(root, absTarget)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cwd escapes root: %s", cwd)
	}

	return absTarget, nil
}

func timeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultTimeout
	}
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

func printableCommand(in RunInput) string {
	if in.UseShell {
		return in.Command
	}
	return strings.Join(append([]string{in.Command}, in.Args...), " ")
}

func mustDecode(raw []byte, v any) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}

	if err := json.Unmarshal(raw, v); err != nil {
		writeError("", "", "", fmt.Errorf("invalid json input: %w", err), 2)
		os.Exit(2)
	}
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(tool, command, dir string, err error, code int) {
	writeJSON(Result{
		OK:       false,
		Tool:     tool,
		Command:  command,
		Dir:      dir,
		ExitCode: code,
		Error:    err.Error(),
	})
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}

	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}

	if len(p) > remaining {
		b.truncated = true
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}

	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
