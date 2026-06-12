package harness

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Protocol-Lattice/go-agent/src/models"
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
		"Every subtask must include enough context",
		"Do not create empty placeholder files",
		"Do not create navigation-only subtasks",
		"Return only numbered lines",
		"Do not use markdown fences",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("buildTaskPlanPrompt() missing %q:\n%s", expected, prompt)
		}
	}
}

func TestBuildTaskPlanPromptRespectsRequestedTaskCount(t *testing.T) {
	prompt := buildTaskPlanPrompt("create simple mcp server in golang in 3 tasks", 5)

	if !strings.Contains(prompt, "exactly 3 numbered subtasks") {
		t.Fatalf("buildTaskPlanPrompt() did not request exactly 3 tasks:\n%s", prompt)
	}
}

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

	if len(ran) != 2 {
		t.Fatalf("ran %d tasks, want 2: %#v", len(ran), ran)
	}
	for i, expected := range []string{"Current subtask:\nfirst task", "Current subtask:\nsecond task"} {
		if !strings.Contains(ran[i], expected) {
			t.Fatalf("ran task %d missing %q:\n%s", i, expected, ran[i])
		}
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

	if len(ran) != 1 {
		t.Fatalf("ran %d tasks, want 1: %#v", len(ran), ran)
	}
	if !strings.Contains(ran[0], "Current subtask:\nfirst task") {
		t.Fatalf("ran task missing first subtask:\n%s", ran[0])
	}
	if !strings.Contains(out.String(), "error: task failed") {
		t.Fatalf("output missing error:\n%s", out.String())
	}
}

func TestTaskLoopRunsSubtasksWithOriginalTaskContext(t *testing.T) {
	ctx := context.Background()
	var ran []string

	loop := taskLoop{
		plan: func(context.Context, string) ([]string, error) {
			return []string{"Write app/main.go with the MCP server implementation"}, nil
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

	in := strings.NewReader("create simple mcp server in golang, write main.go in new folder 'app'\ny\n/exit\n")
	var out bytes.Buffer

	if err := loop.Run(ctx, in, &out); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(ran) != 1 {
		t.Fatalf("ran %d tasks, want 1: %#v", len(ran), ran)
	}

	for _, expected := range []string{
		"Original task:",
		"create simple mcp server in golang",
		"Current subtask:",
		"Write app/main.go with the MCP server implementation",
		"Do not ask follow-up questions",
		"Do not create empty placeholder files",
	} {
		if !strings.Contains(ran[0], expected) {
			t.Fatalf("execution prompt missing %q:\n%s", expected, ran[0])
		}
	}
}

func TestBuildTaskExecutionPrompt(t *testing.T) {
	prompt := buildTaskExecutionPrompt(
		"create simple mcp server in golang, write main.go in new folder 'app'",
		[]string{
			"Create directory app",
			"Write app/main.go with complete MCP server implementation",
		},
		1,
	)

	for _, expected := range []string{
		"Original task:",
		"create simple mcp server in golang",
		"Current subtask:",
		"Write app/main.go with complete MCP server implementation",
		"Full approved task list:",
		"1. Create directory app",
		"2. Write app/main.go with complete MCP server implementation",
		"Do not create empty placeholder files",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("buildTaskExecutionPrompt() missing %q:\n%s", expected, prompt)
		}
	}
}

func TestRuntimePlanTasksUsesModelAndParsesNumberedTasks(t *testing.T) {
	model := &staticTaskPlanModel{
		response: strings.Join([]string{
			"1. Inspect current implementation",
			"2. Add focused tests",
		}, "\n"),
	}
	rt := &Runtime{
		cfg: Config{
			SessionID: "task-loop-test",
			MaxTurns:  5,
			Timeout:   time.Second,
		},
		model: model,
	}

	tasks, err := rt.planTasks(context.Background(), "ship task loop")
	if err != nil {
		t.Fatalf("planTasks returned error: %v", err)
	}

	expected := []string{
		"Inspect current implementation",
		"Add focused tests",
	}
	if !reflect.DeepEqual(tasks, expected) {
		t.Fatalf("planTasks() = %#v, want %#v", tasks, expected)
	}
	if !strings.Contains(model.prompt, "ship task loop") {
		t.Fatalf("planner prompt missing original task:\n%s", model.prompt)
	}
}

func TestRuntimePlanTasksFiltersNavigationAndCapsRequestedCount(t *testing.T) {
	model := &staticTaskPlanModel{
		response: strings.Join([]string{
			"1. Create a new directory named 'app'.",
			"2. Navigate into the 'app' directory.",
			"3. Initialize a new Go module named 'mcp-server' using 'go mod init mcp-server'.",
			"4. Create a file named 'main.go' inside the 'app' directory with a complete simple MCP server implementation.",
			"5. Navigate into the 'app' directory.",
			"6. Run gofmt on app/main.go.",
			"7. Build the app module.",
		}, "\n"),
	}
	rt := &Runtime{
		cfg: Config{
			SessionID: "task-loop-test",
			MaxTurns:  5,
			Timeout:   time.Second,
		},
		model: model,
	}

	tasks, err := rt.planTasks(
		context.Background(),
		"create simple mcp server in golang, write main.go in new folder 'app'. in 3 tasks",
	)
	if err != nil {
		t.Fatalf("planTasks returned error: %v", err)
	}

	if len(tasks) != 3 {
		t.Fatalf("planTasks returned %d tasks, want 3: %#v", len(tasks), tasks)
	}
	for _, task := range tasks {
		if strings.Contains(strings.ToLower(task), "navigate into") {
			t.Fatalf("planTasks kept navigation-only task: %#v", tasks)
		}
	}
	if !strings.Contains(model.prompt, "exactly 3 numbered subtasks") {
		t.Fatalf("planner prompt did not request exactly 3 tasks:\n%s", model.prompt)
	}
}

type staticTaskPlanModel struct {
	response string
	prompt   string
}

func (m *staticTaskPlanModel) Generate(_ context.Context, prompt string) (any, error) {
	m.prompt = prompt
	return m.response, nil
}

func (m *staticTaskPlanModel) GenerateWithFiles(
	ctx context.Context,
	prompt string,
	files []models.File,
) (any, error) {
	return m.Generate(ctx, prompt)
}

func (m *staticTaskPlanModel) GenerateStream(
	ctx context.Context,
	prompt string,
) (<-chan models.StreamChunk, error) {
	response, err := m.Generate(ctx, prompt)
	ch := make(chan models.StreamChunk, 1)
	if err != nil {
		ch <- models.StreamChunk{Err: err}
		close(ch)
		return ch, nil
	}
	ch <- models.StreamChunk{Done: true, FullText: response.(string)}
	close(ch)
	return ch, nil
}
