# TUI Task Loop Design

## Goal

Add a focused task-loop mode to `go-harness tui` that helps users turn one large
task into a short numbered plan, approve that plan, and execute each numbered
task sequentially.

## Existing Context

- `go-harness chat` and `go-harness tui` currently both call the same chat REPL.
- The REPL lives in `internal/harness/runtime.go` as `Runtime.RunREPL`.
- Each non-slash user line currently executes one `Runtime.RunOnce` call.
- `Runtime.RunOnce` already handles session memory, request timeout, model
  generation, transcript fallback, and provider-level tool approval.
- `Config.MaxTurns` exists but is not currently used by the runtime loop.
- `code-review-graph` impact analysis identifies the relevant code surface as
  `internal/harness/runtime.go`, `internal/cli/cli.go`,
  `internal/cli/commands.go`, `internal/harness/prompt.go`, and tests/docs.

## Behavior

`go-harness chat` keeps the current free-form REPL behavior.

`go-harness tui` starts a line-oriented task-loop UI:

1. Print a ready/help message for task-loop mode.
2. Prompt for a larger task.
3. Keep slash commands available for operational controls such as `/help`,
   `/tools`, `/skills`, `/approve`, `/noapprove`, and `/exit`.
4. For a normal task line, ask the model to break the request into `2-5`
   numbered subtasks.
5. Parse the model response into a clean ordered list.
6. Print the numbered subtasks.
7. Ask `run tasks? [y/N]`.
8. If the user answers no or enters anything other than yes, do not run the
   tasks and return to the task prompt.
9. If the user answers yes, run each subtask sequentially with `Runtime.RunOnce`.
10. Print progress before each task as `[current/total] <task>`.
11. Stop on the first task error, print the error, and return to the prompt.

The loop does not run tasks in parallel and does not persist a task queue.

## Task Planning Prompt

The task planner prompt should be deterministic and easy to parse. It should ask
the model to return only numbered task lines, each with one concrete action. The
prompt should preserve the original user request and instruct the model to avoid
extra prose, markdown fences, or nested bullets.

If the model returns no parseable numbered tasks, the TUI should report the
failure and return to the prompt without executing anything.

## Architecture

Add a separate runtime method for task-loop TUI mode named
`Runtime.RunTaskLoop`.

Keep the current REPL method for chat mode. Update `internal/cli` dispatch so:

- `chat` and the empty command keep calling the current chat REPL.
- `tui` calls the new task-loop method.

Introduce small helper functions in `internal/harness` for:

- building the task-planning prompt;
- parsing numbered task lines from model output;
- asking for yes/no approval;
- executing a list of tasks with progress output.

The helpers should stay unexported unless tests need a public seam. Prefer
package-level tests in `internal/harness` so unexported helpers can be tested
without expanding public API.

## Error Handling

- Empty task input should be ignored, matching the current REPL.
- Planner generation errors should be printed and should not terminate the TUI
  session.
- Unparseable planner responses should be printed as an error and should not
  execute.
- User denial should not be an error.
- Task execution should stop on the first `RunOnce` error and print it.
- Scanner errors should still be returned from the loop.

## Testing

Use test-first implementation.

Add focused tests in `internal/harness` for:

- parsing common numbered-list formats;
- rejecting output with no numbered tasks;
- task-loop denial after plan preview;
- task-loop approval executing tasks in order;
- task-loop stopping on the first execution error.

Existing `RunOnce` memory tests should remain unchanged.

## Non-Goals

- No Bubble Tea or full-screen terminal dependency.
- No parallel task execution.
- No persisted task queue.
- No task retry UI.
- No change to provider binaries.
- No change to tool approval policy.
- No change to `go-harness run`.

## Acceptance Criteria

- `go-harness chat` retains current behavior.
- `go-harness tui` previews a numbered task plan and asks before execution.
- Approved tasks execute sequentially with progress output.
- Denied tasks do not execute.
- Execution stops on first task failure.
- New behavior has unit tests.
- `rtk gofmt -w .`, `rtk go test ./...`, and `rtk go vet ./...` pass or any
  failures are reported exactly.
