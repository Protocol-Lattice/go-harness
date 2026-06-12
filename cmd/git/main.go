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
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Inputs      map[string]any `json:"inputs,omitempty"`
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

type baseInput struct {
	CWD     string `json:"cwd,omitempty"`
	Timeout int    `json:"timeout_seconds,omitempty"`
}

type statusInput struct {
	baseInput
	Short bool `json:"short,omitempty"`
}

type diffInput struct {
	baseInput
	Staged   bool     `json:"staged,omitempty"`
	Pathspec []string `json:"pathspec,omitempty"`
}

type addInput struct {
	baseInput
	Paths []string `json:"paths"`
}

type commitInput struct {
	baseInput
	Message string `json:"message"`
	All     bool   `json:"all,omitempty"`
}

type logInput struct {
	baseInput
	Limit int `json:"limit,omitempty"`
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

	if len(args) < 2 {
		writeError(args[0], "", "", errors.New("missing json input argument"), 2)
		os.Exit(2)
	}

	toolName := args[0]
	rawInput := []byte(args[1])

	switch toolName {
	case "git.status":
		var in statusInput
		mustDecode(rawInput, &in)
		runGit(toolName, root, in.CWD, timeout(in.Timeout), statusArgs(in.Short))

	case "git.diff":
		var in diffInput
		mustDecode(rawInput, &in)
		runGit(toolName, root, in.CWD, timeout(in.Timeout), diffArgs(in))

	case "git.add":
		var in addInput
		mustDecode(rawInput, &in)
		if len(in.Paths) == 0 {
			writeError(toolName, "", "", errors.New("paths is required"), 2)
			os.Exit(2)
		}
		runGit(toolName, root, in.CWD, timeout(in.Timeout), append([]string{"add", "--"}, in.Paths...))

	case "git.commit":
		var in commitInput
		mustDecode(rawInput, &in)
		if strings.TrimSpace(in.Message) == "" {
			writeError(toolName, "", "", errors.New("message is required"), 2)
			os.Exit(2)
		}

		args := []string{"commit", "-m", in.Message}
		if in.All {
			args = []string{"commit", "-am", in.Message}
		}
		runGit(toolName, root, in.CWD, timeout(in.Timeout), args)

	case "git.log":
		var in logInput
		mustDecode(rawInput, &in)
		limit := in.Limit
		if limit <= 0 || limit > 50 {
			limit = 10
		}
		runGit(toolName, root, in.CWD, timeout(in.Timeout), []string{
			"log",
			fmt.Sprintf("-%d", limit),
			"--oneline",
			"--decorate",
		})

	default:
		writeError(toolName, "", "", fmt.Errorf("unknown tool %q", toolName), 2)
		os.Exit(2)
	}
}

func parseArgs(args []string) (string, []string, error) {
	fs := flag.NewFlagSet("git-provider", flag.ContinueOnError)
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
			Name:        "git.status",
			Description: "Show git working tree status.",
			Inputs: map[string]any{
				"cwd":             "optional working directory relative to root",
				"short":           "optional bool; use --short output",
				"timeout_seconds": "optional command timeout",
			},
		},
		{
			Name:        "git.diff",
			Description: "Show git diff. Supports staged and optional pathspec.",
			Inputs: map[string]any{
				"cwd":             "optional working directory relative to root",
				"staged":          "optional bool; use --staged",
				"pathspec":        "optional list of paths",
				"timeout_seconds": "optional command timeout",
			},
		},
		{
			Name:        "git.add",
			Description: "Stage files with git add.",
			Inputs: map[string]any{
				"cwd":             "optional working directory relative to root",
				"paths":           "required list of paths",
				"timeout_seconds": "optional command timeout",
			},
		},
		{
			Name:        "git.commit",
			Description: "Create a git commit.",
			Inputs: map[string]any{
				"cwd":             "optional working directory relative to root",
				"message":         "required commit message",
				"all":             "optional bool; use git commit -am",
				"timeout_seconds": "optional command timeout",
			},
		},
		{
			Name:        "git.log",
			Description: "Show recent git commits.",
			Inputs: map[string]any{
				"cwd":             "optional working directory relative to root",
				"limit":           "optional commit limit, max 50",
				"timeout_seconds": "optional command timeout",
			},
		},
	}
}

func statusArgs(short bool) []string {
	if short {
		return []string{"status", "--short"}
	}
	return []string{"status"}
}

func diffArgs(in diffInput) []string {
	args := []string{"diff"}
	if in.Staged {
		args = append(args, "--staged")
	}
	if len(in.Pathspec) > 0 {
		args = append(args, "--")
		args = append(args, in.Pathspec...)
	}
	return args
}

func runGit(toolName, root, cwd string, timeout time.Duration, gitArgs []string) {
	dir, err := safeDir(root, cwd)
	if err != nil {
		writeError(toolName, "git "+strings.Join(gitArgs, " "), "", err, 2)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	result := Result{
		OK:      err == nil,
		Tool:    toolName,
		Command: "git " + strings.Join(gitArgs, " "),
		Dir:     dir,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
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

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
