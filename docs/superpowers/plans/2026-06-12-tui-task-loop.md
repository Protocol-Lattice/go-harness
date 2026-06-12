# TUI Task Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a line-oriented `go-harness tui` task loop that breaks one large task into numbered subtasks, asks for approval, and runs approved tasks sequentially.

**Architecture:** Keep `chat` on the existing REPL and route `tui` to a new `Runtime.RunTaskLoop`. Store the configured model on `Runtime` so task planning can call the model directly without invoking tool orchestration or writing planner chatter into session memory. Add unexported helper functions for planner prompt construction, numbered-list parsing, yes/no parsing, and loop execution. Use an unexported `taskLoop` struct with function fields so unit tests can verify loop behavior without calling a real LLM or provider.

**Tech Stack:** Go standard library, existing `go-agent` runtime, existing `internal/harness` tests, `rtk go test`, `rtk go vet`.

---

## Files

- Modify: `internal/harness/runtime.go`
- Create: `internal/harness/task_loop_test.go`
- Modify: `internal/cli/commands.go`
- Modify: `internal/cli/cli.go`
- Modify: `README.md`

## Task 1: Add Task Planning Helpers

**Files:**
- Modify: `internal/harness/runtime.go`
- Create: `internal/harness/task_loop_test.go`

- [ ] **Step 1: Write failing parser and prompt tests**

Add `internal/harness/task_loop_test.go`:

```go
package harness

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseNumberedTasks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name: "plain numbered lines",
			input: strings.Join([]string{
				"1. Inspect current TUI flow",
				"2. Add task loop tests",
				"3. Implement task loop",
			}, "\n"),
			expected: []string{
				"Inspect current TUI flow",
				"Add task loop tests",
				"Implement task loop",
			},
		},
		{
			name: "parenthesized numbers and whitespace",
			input: strings.Join([]string{
				"  1) Read the CLI dispatch  ",
				"  2) Wire tui mode",
			}, "\n"),
			expected: []string{
				"Read the CLI dispatch",
				"Wire tui mode",
			},
		},
		{
			name: "ignores prose around list",
			input: strings.Join([]string{
				"Here is the plan:",
				"1. Add tests",
				"2. Add implementation",
				"Done.",
			}, "\n"),
			expected: []string{
				"Add tests",
				"Add implementation",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNumberedTasks(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("parseNumberedTasks() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestParseNumberedTasksRejectsNoTasks(t *testing.T) {
	got := parseNumberedTasks("inspect files\nwrite tests\nship it")
	if len(got) != 0 {
		t.Fatalf("parseNumberedTasks() = %#v, want empty slice", got)
	}
}

func TestBuildTaskPlanPrompt(t *testing.T) {
	prompt := buildTaskPlanPrompt("fix the tui loop", 5)

	for _, expected := range []string{
		"fix the tui loop",
		"2-5 numbered subtasks",
		"Return only numbered lines",
		"Do not use markdown fences",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("buildTaskPlanPrompt() missing %q:\n%s", expected, prompt)
		}
	}
}
```

- [ ] **Step 2: Run helper tests to verify failure**

Run:

```sh
rtk go test ./internal/harness -run 'TestParseNumberedTasks|TestBuildTaskPlanPrompt'
```

Expected: FAIL because `parseNumberedTasks` and `buildTaskPlanPrompt` are undefined.

- [ ] **Step 3: Add minimal helpers**

In `internal/harness/runtime.go`, add imports:

```go
import (
	"regexp"
)
```

Keep existing imports and add this package-level regexp near constants:

```go
var numberedTaskLinePattern = regexp.MustCompile(`^\s*\d+[\.)]\s+(.+?)\s*$`)
```

Add helpers near the REPL methods:

```go
func buildTaskPlanPrompt(task string, maxTasks int) string {
	if maxTasks < 2 {
		maxTasks = 2
	}

	return fmt.Sprintf(`Break this task into 2-%d numbered subtasks.

Original task:
%s

Rules:
- Return only numbered lines.
- Each line must be one concrete action.
- Do not use markdown fences.
- Do not use nested bullets.
- Do not include extra prose.`, maxTasks, strings.TrimSpace(task))
}

func parseNumberedTasks(output string) []string {
	tasks := []string{}
	for _, line := range strings.Split(output, "\n") {
		matches := numberedTaskLinePattern.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}

		task := strings.TrimSpace(matches[1])
		if task == "" {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func maxPlannedTasks(cfg Config) int {
	if cfg.MaxTurns < 2 {
		return 2
	}
	if cfg.MaxTurns > 5 {
		return 5
	}
	return cfg.MaxTurns
}
```

- [ ] **Step 4: Run helper tests to verify pass**

Run:

```sh
rtk go test ./internal/harness -run 'TestParseNumberedTasks|TestBuildTaskPlanPrompt'
```

Expected: PASS.

## Task 2: Add Task Loop Core

**Files:**
- Modify: `internal/harness/runtime.go`
- Modify: `internal/harness/task_loop_test.go`

- [ ] **Step 1: Write failing loop tests**

Append to `internal/harness/task_loop_test.go`:

```go
import (
	"bytes"
	"context"
	"errors"
	"io"
)
```

Merge these with existing imports, then add:

```go
func TestTaskLoopDenialDoesNotRunTasks(t *testing.T) {
	ctx := context.Background()
	var ran []string

	loop := taskLoop{
		plan: func(context.Context, string) ([]string, error) {
			return []string{"first task", "second task"}, nil
		},
		run: func(_ context.Context, task string, _ io.Writer) error {
			ran = append(ran, task)
			return nil
		},
		slash: func(context.Context, string, io.Writer) error {
			return nil
		},
		printHelp: func(io.Writer) {},
	}

	in := strings.NewReader("large task\nn\n/exit\n")
	var out bytes.Buffer

	if err := loop.Run(ctx, in, &out); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(ran) != 0 {
		t.Fatalf("ran tasks after denial: %#v", ran)
	}
	if !strings.Contains(out.String(), "1. first task") {
		t.Fatalf("output missing planned task:\n%s", out.String())
	}
}

func TestTaskLoopApprovalRunsTasksInOrder(t *testing.T) {
	ctx := context.Background()
	var ran []string

	loop := taskLoop{
		plan: func(context.Context, string) ([]string, error) {
			return []string{"first task", "second task"}, nil
		},
		run: func(_ context.Context, task string, _ io.Writer) error {
			ran = append(ran, task)
			return nil
		},
		slash: func(context.Context, string, io.Writer) error {
			return nil
		},
		printHelp: func(io.Writer) {},
	}

	in := strings.NewReader("large task\ny\n/exit\n")
	var out bytes.Buffer

	if err := loop.Run(ctx, in, &out); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	expected := []string{"first task", "second task"}
	if !reflect.DeepEqual(ran, expected) {
		t.Fatalf("ran tasks = %#v, want %#v", ran, expected)
	}
	if !strings.Contains(out.String(), "[1/2] first task") {
		t.Fatalf("output missing progress:\n%s", out.String())
	}
}

func TestTaskLoopStopsOnFirstTaskError(t *testing.T) {
	ctx := context.Background()
	var ran []string
	taskErr := errors.New("task failed")

	loop := taskLoop{
		plan: func(context.Context, string) ([]string, error) {
			return []string{"first task", "second task"}, nil
		},
		run: func(_ context.Context, task string, _ io.Writer) error {
			ran = append(ran, task)
			return taskErr
		},
		slash: func(context.Context, string, io.Writer) error {
			return nil
		},
		printHelp: func(io.Writer) {},
	}

	in := strings.NewReader("large task\ny\n/exit\n")
	var out bytes.Buffer

	if err := loop.Run(ctx, in, &out); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	expected := []string{"first task"}
	if !reflect.DeepEqual(ran, expected) {
		t.Fatalf("ran tasks = %#v, want %#v", ran, expected)
	}
	if !strings.Contains(out.String(), "error: task failed") {
		t.Fatalf("output missing error:\n%s", out.String())
	}
}
```

- [ ] **Step 2: Run loop tests to verify failure**

Run:

```sh
rtk go test ./internal/harness -run 'TestTaskLoop'
```

Expected: FAIL because `taskLoop` is undefined.

- [ ] **Step 3: Add task loop implementation**

In `internal/harness/runtime.go`, add:

```go
type taskLoop struct {
	plan      func(context.Context, string) ([]string, error)
	run       func(context.Context, string, io.Writer) error
	slash     func(context.Context, string, io.Writer) error
	printHelp func(io.Writer)
}

func (l taskLoop) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)

	fmt.Fprintln(out, "agentic-tui task loop ready")
	l.printHelp(out)
	fmt.Fprintln(out)

	for {
		fmt.Fprint(out, "task ❯ ")

		if !scanner.Scan() {
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			if err := l.slash(ctx, line, out); err != nil {
				if err == errExitREPL {
					return nil
				}
				fmt.Fprintf(out, "error: %v\n", err)
			}
			continue
		}

		tasks, err := l.plan(ctx, line)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}
		if len(tasks) == 0 {
			fmt.Fprintln(out, "error: no numbered tasks found")
			continue
		}

		printPlannedTasks(out, tasks)
		fmt.Fprint(out, "run tasks? [y/N] ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		if !isYes(scanner.Text()) {
			fmt.Fprintln(out, "skipped")
			continue
		}

		runTasks(ctx, tasks, out, l.run)
	}
}

func printPlannedTasks(out io.Writer, tasks []string) {
	for i, task := range tasks {
		fmt.Fprintf(out, "%d. %s\n", i+1, task)
	}
}

func isYes(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func runTasks(
	ctx context.Context,
	tasks []string,
	out io.Writer,
	run func(context.Context, string, io.Writer) error,
) {
	for i, task := range tasks {
		fmt.Fprintf(out, "[%d/%d] %s\n", i+1, len(tasks), task)
		if err := run(ctx, task, out); err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return
		}
	}
}
```

- [ ] **Step 4: Run loop tests to verify pass**

Run:

```sh
rtk go test ./internal/harness -run 'TestTaskLoop'
```

Expected: PASS.

## Task 3: Wire Runtime and CLI

**Files:**
- Modify: `internal/harness/runtime.go`
- Modify: `internal/cli/commands.go`
- Modify: `internal/cli/cli.go`

- [ ] **Step 1: Write failing CLI/runtime tests if existing seams allow**

If adding a CLI test would require constructing a real `Runtime`, skip CLI unit
tests and rely on direct helper tests plus `go test ./...`. Do not add network
or provider-dependent tests.

- [ ] **Step 2: Add runtime methods**

In `internal/harness/runtime.go`, add `errors` to imports and add a `model`
field to `Runtime`:

```go
import (
	"errors"
)
```

```go
type Runtime struct {
	cfg         Config
	agent       *agent.Agent
	model       models.Agent
	gate        *ApprovalGate
	memoryStore *memory.MarkdownStore
}
```

In `NewRuntime`, set the field:

```go
return &Runtime{
	cfg:         cfg,
	agent:       a,
	model:       model,
	memoryStore: mdStore,
	gate:        gate,
}, nil
```

Then add:

```go
func (r *Runtime) RunTaskLoop(ctx context.Context, in io.Reader, out io.Writer) error {
	loop := taskLoop{
		plan:      r.planTasks,
		run:       r.RunOnce,
		slash:     r.runSlashCommand,
		printHelp: r.printTaskLoopHelp,
	}
	return loop.Run(ctx, in, out)
}

func (r *Runtime) planTasks(ctx context.Context, task string) ([]string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	if r.model == nil {
		return nil, errors.New("task planner model is not configured")
	}

	resp, err := r.model.Generate(reqCtx, buildTaskPlanPrompt(task, maxPlannedTasks(r.cfg)))
	if err != nil {
		return nil, err
	}
	return parseNumberedTasks(fmt.Sprint(resp)), nil
}

func (r *Runtime) printTaskLoopHelp(out io.Writer) {
	fmt.Fprintln(out, "commands: /help, /exit, /tools [list], /skills [list], /approve, /noapprove, /run <prompt>")
}
```

- [ ] **Step 3: Add CLI `tui` command method**

In `internal/cli/commands.go`, add:

```go
func (a *App) tui(ctx context.Context, args []string) error {
	cfg, _, err := a.baseFlags("tui", args)
	if err != nil {
		return err
	}

	rt, err := harness.NewRuntime(ctx, cfg, a.stdin, a.stdout)
	if err != nil {
		return err
	}

	return rt.RunTaskLoop(ctx, a.stdin, a.stdout)
}
```

- [ ] **Step 4: Route `go-harness tui` to task loop**

In `internal/cli/cli.go`, change:

```go
case "tui":
	return app.chat(ctx, args[1:])
```

to:

```go
case "tui":
	return app.tui(ctx, args[1:])
```

- [ ] **Step 5: Run focused tests**

Run:

```sh
rtk go test ./internal/harness ./internal/cli
```

Expected: PASS.

## Task 4: Update Docs and Full Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README TUI description**

In `README.md`, update the usage area to say that `go-harness tui` opens the
task-loop UI and `go-harness chat` opens the free-form REPL. Keep command
examples short.

- [ ] **Step 2: Run gofmt**

Run:

```sh
rtk gofmt -w .
```

Expected: no output.

- [ ] **Step 3: Run full tests**

Run:

```sh
rtk go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run vet**

Run:

```sh
rtk go vet ./...
```

Expected: PASS.

- [ ] **Step 5: Run code-review-graph impact context**

Run `code-review-graph` MCP for changed files:

```text
get_impact_radius_tool(repo_root="/Users/raezil/Desktop/go-harness", changed_files=["internal/harness/runtime.go","internal/harness/task_loop_test.go","internal/cli/commands.go","internal/cli/cli.go","README.md"], max_depth=2)
```

Expected: impact remains limited to CLI/runtime/test/doc surfaces.
