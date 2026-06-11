# go-harness

Agentic Go coding harness built around [`go-agent`](https://github.com/Protocol-Lattice/go-agent) and [`go-utcp`](https://github.com/universal-tool-calling-protocol/go-utcp).

`go-harness` is a lightweight command-line coding assistant for Go projects. It loads markdown skills, registers UTCP tool providers, keeps markdown-backed session memory, and can run in either interactive chat mode or one-shot task mode.

## Features

* Interactive agentic coding REPL
* One-shot task execution from the CLI
* UTCP-backed tool discovery and execution
* Local filesystem provider example
* Markdown skill loading from `./skills`
* GitHub skill fetching from public repositories
* Markdown-backed session memory
* Approval gate for tool/code execution
* `doctor` command for local setup checks
* Go-focused system prompt with gofmt/test guidance

## Repository layout

```text
.
├── cmd/
│   ├── go-harness/      # Main CLI entrypoint
│   └── filesystem/      # Example filesystem UTCP CLI provider
├── internal/
│   ├── cli/             # CLI commands, flags, doctor, version
│   └── harness/         # Runtime, skills, prompts, approvals
├── Makefile
├── go.mod
└── README.md
```

## Requirements

* Go 1.26+
* An LLM provider supported by `go-agent`
* Optional: `GITHUB_TOKEN` for fetching skills from private repositories or avoiding GitHub API rate limits

## Install

Clone the repository:

```bash
git clone https://github.com/Protocol-Lattice/go-harness.git
cd go-harness
```

Build the main CLI:

```bash
make build
```

Build the example filesystem provider:

```bash
make build-filesystem
```

Or build everything needed for local checks:

```bash
make doctor
```

Install the CLI into your Go bin path:

```bash
make install
```

## Quick start

Start interactive mode:

```bash
./bin/go-harness
```

Run a one-shot task:

```bash
./bin/go-harness run "inspect this Go project and suggest the next small refactor"
```

Shortcut form:

```bash
./bin/go-harness fix tests
```

Enable auto-approval for tool/code execution:

```bash
./bin/go-harness -y refactor project layout
```

Use a different model:

```bash
./bin/go-harness chat -provider openai -model gpt-4o-mini
```

## Commands

```bash
go-harness [prompt...]
go-harness chat [flags]
go-harness run [flags] <task>
go-harness doctor [flags]
go-harness skills list [flags]
go-harness skills fetch [flags] <github-source>
go-harness tools list [flags]
go-harness version
```

## Common flags

| Flag         |            Default | Description                          |
| ------------ | -----------------: | ------------------------------------ |
| `-provider`  |           `openai` | LLM provider                         |
| `-model`     |      `gpt-4o-mini` | Model name                           |
| `-session`   |       `go-harness` | Memory session ID                    |
| `-skills`    |         `./skills` | Directory containing markdown skills |
| `-providers` | `./providers.json` | UTCP providers file                  |
| `-workspace` |                `.` | Workspace root                       |
| `-max-turns` |                `8` | Maximum autonomous turns             |
| `-timeout`   |               `2m` | Per-request timeout                  |
| `-memory`    |    `.agent-memory` | Markdown memory directory            |
| `-y`         |            `false` | Auto-approve tool/code execution     |

## Provider configuration

`go-harness` uses UTCP providers for tool discovery and execution. A minimal local filesystem provider can look like this:

```json
[
  {
    "provider_type": "cli",
    "name": "filesystem",
    "command_name": "./bin/filesystem",
    "discover_command": "./bin/filesystem --root . list-tools",
    "execute_command": "./bin/filesystem --root . {{.ToolName}} {{json .Inputs}}"
  }
]
```

Build the provider first:

```bash
make build-filesystem
```

Then verify that tools are visible:

```bash
./bin/go-harness tools list
```

Expected filesystem tools include:

```text
fs.list
fs.read
fs.write
fs.mkdir
fs.remove
fs.stat
fs.rename
fs.copy
fs.exists
```

## Skills

Skills are markdown files loaded from the skills directory.

List loaded skills:

```bash
./bin/go-harness skills list
```

Fetch skills from a GitHub repository:

```bash
./bin/go-harness skills fetch Protocol-Lattice/go-harness-skills
```

Fetch from a specific subdirectory:

```bash
./bin/go-harness skills fetch https://github.com/Protocol-Lattice/go-harness-skills/tree/main/skills
```

When a GitHub token is needed:

```bash
GITHUB_TOKEN=ghp_xxx ./bin/go-harness skills fetch Protocol-Lattice/go-harness-skills
```

## Doctor

Run local diagnostics:

```bash
./bin/go-harness doctor
```

The doctor command checks:

* workspace path
* skills directory
* providers file
* filesystem provider binary
* loaded skill count

## Development

Run tests:

```bash
make test
```

Tidy modules:

```bash
make tidy
```

Build binaries:

```bash
make build
make build-filesystem
```

## How it works

At startup, `go-harness`:

1. Parses CLI flags.
2. Loads markdown skills from the configured skills directory.
3. Creates the configured LLM provider.
4. Creates markdown-backed session memory.
5. Creates a UTCP client from the providers file.
6. Builds a Go-focused system prompt.
7. Starts either interactive REPL mode or one-shot task mode.

The built-in system prompt tells the agent to prefer small, testable changes, use loaded skills, inspect files through UTCP tools, ask before destructive operations, and avoid claiming changes unless tools confirm them.

## Safety model

`go-harness` includes an approval gate for tool/code execution.

By default, operations requiring approval should ask first. Use `-y` only in trusted repositories or disposable sandboxes:

```bash
./bin/go-harness -y run "create a small hello world app in a new folder"
```

Recommended practice:

* Run inside a git repository.
* Commit before large automated changes.
* Review diffs before keeping changes.
* Avoid `-y` when working near secrets or production files.
* Keep filesystem provider roots scoped to the intended workspace.

## Status

Early-stage experimental harness for Go agent workflows.

The current focus is:

* reliable UTCP provider discovery
* safer tool execution
* practical Go coding workflows
* skill-driven agent behavior
* simple local development ergonomics

## License

Add a `LICENSE` file before publishing releases.
