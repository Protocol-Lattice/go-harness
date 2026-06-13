# go-harness

`go-harness` is a Go-native Claude Code-style coding agent: a toolkit that turns
"LLM wants to modify files" into a safe CodeMode workflow. Built on
[`github.com/Protocol-Lattice/go-agent`](https://github.com/Protocol-Lattice/go-agent),
it provides a CLI/REPL that can load markdown skills, persist session memory,
discover UTCP tools, and run coding tasks through filesystem, shell, and git
provider binaries.

The repository also includes a CodeMode output normalizer. The normalizer repairs
common generated snippets before they reach the strict CodeMode validation path,
including JSON-wrapped snippets, invalid `return __out` exits, incorrect
`shell.run` arguments, and invented `codemode.*` helpers.

## Getting Started

Requirements:

- Go 1.26.3, matching the version declared in `go.mod`
- Git, when using the git provider
- Provider credentials required by the selected LLM provider

Build the harness and bundled provider binaries:

```bash
make build
```

Run the test suite:

```bash
make test
```

Install the `go-harness` binary with `go install`:

```bash
make install
```

## Provider Configuration

`go-harness` expects a UTCP providers file at `./providers.json` unless
`-providers` is set. The checked-in file at `cmd/go-harness/providers.json` is a
template for the three bundled CLI providers, but its command paths are relative
to `cmd/go-harness`.

For normal use from the repository root, create a root-level `providers.json`
with paths that point at the built binaries:

```json
[
  {
    "provider_type": "cli",
    "name": "filesystem",
    "command_name": "./bin/filesystem/filesystem",
    "discover_command": "./bin/filesystem/filesystem --root . list-tools",
    "execute_command": "./bin/filesystem/filesystem --root . {{.ToolName}} {{json .Inputs}}"
  },
  {
    "provider_type": "cli",
    "name": "git",
    "command_name": "./bin/git/git",
    "discover_command": "./bin/git/git --root . list-tools",
    "execute_command": "./bin/git/git --root . {{.ToolName}} {{json .Inputs}}"
  },
  {
    "provider_type": "cli",
    "name": "shell",
    "command_name": "./bin/shell/shell",
    "discover_command": "./bin/shell/shell --root . list-tools",
    "execute_command": "./bin/shell/shell --root . {{.ToolName}} {{json .Inputs}}"
  }
]
```

After building providers and adding the root providers file, verify discovery:

```bash
./bin/go-harness tools list
```

## Usage

Start the free-form interactive REPL:

```bash
./bin/go-harness
```

Start the task-loop TUI, which previews numbered subtasks and asks before
running them:

```bash
./bin/go-harness tui
```

Run a single prompt:

```bash
./bin/go-harness run "add tests for the filesystem provider"
```

Common commands:

```bash
./bin/go-harness chat
./bin/go-harness tui
./bin/go-harness run "fix failing tests"
./bin/go-harness skills list
./bin/go-harness skills fetch Protocol-Lattice/go-harness-skills
./bin/go-harness tools list
./bin/go-harness version
```

The chat REPL and task-loop TUI support slash commands:

```text
/help
/status
/tools
/skills
/approve
/noapprove
/run <prompt>
/exit
```

Useful flags:

| Flag | Default | Purpose |
| --- | --- | --- |
| `-provider` | `openai` | LLM provider passed to `go-agent` |
| `-model` | `gpt-4o-mini` | Model name |
| `-session` | `go-harness` | Memory session ID |
| `-skills` | `./skills` | Directory containing markdown skills |
| `-providers` | `./providers.json` | UTCP provider configuration |
| `-workspace` | `.` | Workspace path included in the system prompt |
| `-max-turns` | `8` | Maximum autonomous turns |
| `-timeout` | `2m` | Per-request timeout |
| `-y` | `false` | Auto-approve tool/code execution |

## Safe CodeMode Workflow

When the LLM needs to modify files, `go-harness` treats the edit as a workflow
instead of raw filesystem access:

- inspect the workspace with read-only filesystem, shell, and git tools;
- group related filesystem, shell, and git calls through `codemode.run_code`
  when one coherent operation needs multiple tool calls;
- route every tool call through the same approval gate before risky operations;
- constrain bundled providers to the configured workspace root;
- validate with shell or git output when relevant, then summarize only
  tool-confirmed results.

This workflow makes file changes reviewable and approval-gated. It does not make
LLM-generated code correct by itself; tests and explicit validation still matter.

## Agent Status

Use `/status` in chat or task-loop mode to inspect the active harness
configuration without exposing credentials:

```text
/status
```

The status output includes the provider, model, workspace, session, skills
directory, providers file, memory directory, max turns, timeout, and current
auto-approval state.

## Tool Approval

`go-harness` enforces provider-level approval before executing risky tools.
Read-only calls such as `filesystem.read`, `filesystem.list`, `git.status`,
`git.diff`, and `git.log` run without prompting.

The harness asks before `shell.run`, mutating filesystem tools, `git.add`,
`git.commit`, streamed calls, and unknown tools. Use `-y` for a non-interactive
run, or `/approve` and `/noapprove` inside the REPL to toggle auto-approval.

## Bundled Providers

The Makefile builds three provider binaries:

- `./bin/filesystem/filesystem` exposes `filesystem.list`, `filesystem.read`,
  `filesystem.write`, `filesystem.mkdir`, `filesystem.remove`,
  `filesystem.stat`, `filesystem.rename`, `filesystem.copy`, and
  `filesystem.exists`.
- `./bin/shell/shell` exposes `shell.run` for non-interactive commands under the
  configured workspace root. It prefers argv execution and only allows shell
  mode when `HARNESS_ALLOW_SHELL=1` is set.
- `./bin/git/git` exposes `git.status`, `git.diff`, `git.add`, `git.commit`,
  and `git.log`.

Each provider accepts `--root` to constrain file, shell, or git operations to a
workspace root.

## CodeMode Normalization

`internal/harness/codemode_normalize.go` wraps the configured model provider and
normalizes generated CodeMode snippets before execution. It handles recurring
failure modes such as:

- JSON objects containing a `"code"` field
- tool-choice JSON for `codemode.run_code`
- fenced snippets that contain `codemode.CallTool(...)`
- `runResult` and `runRes` result aliases
- `shell.run` calls passed as `[]string{...}` instead of a map with `argv`
- missing trailing commas in multiline `map[string]any{...}` literals
- generated Go programs that must be converted into filesystem writes

The normalizer is covered by `internal/harness/codemode_normalize_test.go`.

## Development

Use the Makefile targets for routine work:

```bash
make build
make test
make tidy
```

`make doctor` currently reports `./bin/filesystem` as a directory because the
Makefile builds the provider binary at `./bin/filesystem/filesystem`. Until that
check is updated, prefer `make build`, `make test`, and `./bin/go-harness tools list`
with a root-local provider file for validation.

## Repository Layout

```text
cmd/go-harness/     CLI entry point and provider template
cmd/filesystem/     Filesystem UTCP CLI provider
cmd/shell/          Shell UTCP CLI provider
cmd/git/            Git UTCP CLI provider
internal/cli/       Command parsing and user-facing CLI commands
internal/harness/   Runtime, prompts, skills, memory, approval, normalization
bin/                Built binaries
```

## License

No license file is included in this repository.
