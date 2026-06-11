package harness

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	agent "github.com/Protocol-Lattice/go-agent"
	"github.com/Protocol-Lattice/go-agent/src/memory"
	"github.com/Protocol-Lattice/go-agent/src/models"
	"github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/plugins/codemode"
)

type Runtime struct {
	cfg   Config
	agent *agent.Agent
	gate  ApprovalGate
}

// internal/harness/runtime.go

func NewRuntime(ctx context.Context, cfg Config, stdin io.Reader, stdout io.Writer) (*Runtime, error) {
	skills, err := LoadSkills(ctx, cfg.SkillsDir)
	if err != nil {
		return nil, err
	}

	baseModel, err := models.NewLLMProvider(ctx, cfg.Provider, cfg.Model, "")
	if err != nil {
		return nil, fmt.Errorf("create model provider: %w", err)
	}
	model := newNormalizingModel(baseModel)

	mdStore, err := memory.NewMarkdownStore(cfg.MemoryDir)
	if err != nil {
		return nil, fmt.Errorf("create markdown memory store: %w", err)
	}

	mem := memory.NewSessionMemory(
		memory.NewMemoryBankWithStore(mdStore),
		16,
	)
	client, err := utcp.NewUTCPClient(
		context.Background(), &utcp.UtcpClientConfig{
			ProvidersFilePath: cfg.ProvidersFile,
		},
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	systemPrompt := BuildSystemPrompt(skills, cfg.Workspace)

	a, err := agent.New(agent.Options{
		Model:        model,
		Memory:       mem,
		SystemPrompt: systemPrompt,
		UTCPClient:   client,
		CodeMode:     codemode.NewCodeModeUTCP(client, model),

		AllowUnsafeTools: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return &Runtime{
		cfg:   cfg,
		agent: a,
		gate: ApprovalGate{
			AutoApprove: cfg.AutoApprove,
			In:          stdin,
			Out:         stdout,
		},
	}, nil
}

func (r *Runtime) RunREPL(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)

	fmt.Fprintln(out, "agentic-tui ready")
	r.printREPLHelp(out)
	fmt.Fprintln(out)

	for {
		fmt.Fprint(out, "❯ ")

		if !scanner.Scan() {
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			if err := r.runSlashCommand(ctx, line, out); err != nil {
				if err == errExitREPL {
					return nil
				}
				fmt.Fprintf(out, "error: %v\n", err)
			}
			continue
		}

		if err := r.RunOnce(ctx, line, out); err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
		}
	}
}

func (r *Runtime) runSlashCommand(ctx context.Context, line string, out io.Writer) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}

	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "/exit", "/quit", "/q":
		return errExitREPL

	case "/help", "/h", "/?":
		r.printREPLHelp(out)
		return nil

	case "/approve":
		r.gate.AutoApprove = true
		fmt.Fprintln(out, "auto-approve enabled")
		return nil

	case "/noapprove", "/no-approve":
		r.gate.AutoApprove = false
		fmt.Fprintln(out, "auto-approve disabled")
		return nil

	case "/tools":
		if len(args) > 0 && args[0] != "list" {
			return fmt.Errorf("usage: /tools [list]")
		}
		return r.ListTools(out)

	case "/skills":
		if len(args) > 0 && args[0] != "list" {
			return fmt.Errorf("usage: /skills [list]")
		}
		return r.ListSkills(ctx, out)

	case "/run":
		prompt := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		if prompt == "" {
			return fmt.Errorf("usage: /run <prompt>")
		}
		return r.RunOnce(ctx, prompt, out)

	default:
		return fmt.Errorf("unknown command %q; use /help", fields[0])
	}
}

func (r *Runtime) printREPLHelp(out io.Writer) {
	fmt.Fprintln(out, "commands: /help, /exit, /tools [list], /skills [list], /approve, /noapprove, /run <prompt>")
}

var errExitREPL = fmt.Errorf("exit repl")

func Run(ctx context.Context, cfg Config) error {
	rt, err := NewRuntime(ctx, cfg, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}

	return rt.RunREPL(ctx, os.Stdin, os.Stdout)
}

func (r *Runtime) RunOnce(ctx context.Context, prompt string, out io.Writer) error {
	reqCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	resp, err := r.agent.Generate(reqCtx, r.cfg.SessionID, prompt)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, strings.TrimSpace(fmt.Sprint(resp)))
	return nil
}

func (r *Runtime) ListTools(out io.Writer) error {
	tools, err := r.agent.UTCPClient.SearchTools("", 100)
	if err != nil {
		return err
	}
	if len(tools) == 0 {
		fmt.Fprintln(out, "no tools registered")
		return nil
	}

	for _, t := range tools {
		if t.Description == "" {
			fmt.Fprintf(out, "- %s\n", t.Name)
			continue
		}
		fmt.Fprintf(out, "- %s\n  %s\n", t.Name, t.Description)
	}

	return nil
}

func (r *Runtime) ListSkills(ctx context.Context, out io.Writer) error {
	skills, err := LoadSkills(ctx, r.cfg.SkillsDir)
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		fmt.Fprintln(out, "no skills loaded")
		return nil
	}

	for _, s := range skills {
		if s.Description == "" {
			fmt.Fprintf(out, "- %s\t%s\n", s.Name, s.Path)
			continue
		}
		fmt.Fprintf(out, "- %s\t%s\n  %s\n", s.Name, s.Path, s.Description)
	}

	return nil
}
