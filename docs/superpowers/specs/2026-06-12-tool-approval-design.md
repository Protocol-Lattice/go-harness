# Tool Approval Design

## Goal

Add provider-level tool approval to `go-harness` so read-only tool calls stay fast,
while mutating, shell, git staging/commit, and stream calls require user approval
unless auto-approval is enabled.

## Existing Context

- `internal/harness/approval.go` already defines `ApprovalGate`, but it is not
  wired into tool execution.
- `internal/harness/runtime.go` creates one UTCP client and passes it to both
  `agent.New` and `codemode.NewCodeModeUTCP`.
- `go-agent` exposes no approval hook in `agent.Options`.
- CodeMode helper calls use the configured `utcp.UtcpClientInterface`, so a
  client wrapper can cover normal tool calls and CodeMode nested tool calls.

## Behavior

Tool calls are classified by tool name:

- Allowed without prompt: `filesystem.read`, `filesystem.list`,
  `filesystem.stat`, `filesystem.exists`, `git.status`, `git.diff`, `git.log`.
- Approval required: `filesystem.write`, `filesystem.mkdir`,
  `filesystem.rename`, `filesystem.copy`, `filesystem.remove`, `git.add`,
  `git.commit`, `shell.run`, every streamed call.
- Unknown tools require approval.
- `-y` and `/approve` bypass prompts.
- `/noapprove` restores prompting.
- Denial returns `tool call denied: <tool>`.

Approval prompts include the tool name and compact JSON arguments so the user can
make an informed decision.

## Architecture

Create a small wrapper around `utcp.UtcpClientInterface` in `internal/harness`.
The wrapper delegates discovery and provider registration to the inner client,
but checks policy before `CallTool` and `CallToolStream`.

`Runtime` will hold a pointer to `ApprovalGate`. `NewRuntime` will build the
gate first, wrap the UTCP client with that same gate pointer, and pass the
wrapped client to both the agent and CodeMode. Slash commands mutate the same
gate pointer so approvals change at runtime.

## Testing

Use unit tests with a stub UTCP client:

- Read-only tools bypass approval and call the inner client.
- Mutating tools prompt and are denied when the user answers no.
- Auto-approval bypasses prompting and calls the inner client.
- Stream calls require approval even for otherwise read-only tools.

Then run `go test ./internal/harness` and `go test ./...`.
