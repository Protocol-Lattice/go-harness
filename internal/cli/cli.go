package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Protocol-Lattice/go-harness/internal/harness"
)

type App struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	app := &App{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	if len(args) == 0 {
		return app.chat(ctx, nil)
	}

	switch args[0] {
	case "chat":
		return app.chat(ctx, args[1:])

	case "tui":
		return app.chat(ctx, args[1:])

	case "run":
		return app.runTask(ctx, args[1:])

	case "doctor":
		return app.doctor(ctx, args[1:])

	case "skills":
		return app.skills(ctx, args[1:])

	case "tools":
		return app.tools(ctx, args[1:])

	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "go-harness %s commit=%s date=%s\n", Version, Commit, Date)
		return nil

	case "help", "--help", "-h":
		app.printHelp()
		return nil

	default:
		// JCode/Codex-style shortcut:
		// `go-harness fix tests` means `go-harness run "fix tests"`.
		return app.runTask(ctx, args)
	}
}

func (a *App) baseFlags(name string, args []string) (harness.Config, *flag.FlagSet, error) {
	cfg := harness.Config{
		Provider:      "openai",
		Model:         "gpt-4o-mini",
		SessionID:     "go-harness",
		SkillsDir:     "./skills",
		ProvidersFile: "./providers.json",
		Workspace:     ".",
		MemoryDir:     ".agent-memory",
		MaxTurns:      8,
		Timeout:       2 * time.Minute,
		AutoApprove:   false,
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)

	fs.StringVar(&cfg.Provider, "provider", cfg.Provider, "LLM provider")
	fs.StringVar(&cfg.Model, "model", cfg.Model, "model name")
	fs.StringVar(&cfg.SessionID, "session", cfg.SessionID, "memory session id")
	fs.StringVar(&cfg.SkillsDir, "skills", cfg.SkillsDir, "skills directory")
	fs.StringVar(&cfg.ProvidersFile, "providers", cfg.ProvidersFile, "UTCP providers file")
	fs.StringVar(&cfg.Workspace, "workspace", cfg.Workspace, "workspace root")
	fs.StringVar(&cfg.MemoryDir, "memory", cfg.MemoryDir, "markdown memory directory")
	fs.IntVar(&cfg.MaxTurns, "max-turns", cfg.MaxTurns, "max autonomous turns")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "request timeout")
	fs.BoolVar(&cfg.AutoApprove, "y", cfg.AutoApprove, "auto-approve tool/code execution")

	if err := fs.Parse(args); err != nil {
		return cfg, fs, err
	}

	return cfg, fs, nil
}
func (a *App) printHelp() {
	fmt.Fprintln(a.stdout, strings.TrimSpace(`
go-harness - agentic Go coding harness

Usage:
  go-harness [prompt...]
  go-harness chat [flags]
  go-harness tui [flags]
  go-harness run [flags] <prompt>
  go-harness doctor [flags]
  go-harness skills list [flags]
  go-harness skills fetch [flags] <github-url-or-owner/repo>
  go-harness tools list [flags]
  go-harness version

Examples:
  go-harness
  go-harness tui
  go-harness run "add ./bin/filesystem"
  go-harness chat -model gpt-4o-mini
  go-harness doctor
  go-harness skills list
  go-harness skills fetch Protocol-Lattice/go-harness-skills
  go-harness skills fetch https://github.com/Protocol-Lattice/go-harness-skills/tree/main/skills
  go-harness -y refactor project layout
`))
}
