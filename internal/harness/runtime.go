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

	model, err := models.NewLLMProvider(ctx, cfg.Provider, cfg.Model, "")
	if err != nil {
		return nil, fmt.Errorf("create model provider: %w", err)
	}

	// ADD IT HERE
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
			ProvidersFilePath: "providers.json",
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
	fmt.Fprintln(out, "commands: /exit, /tools, /skills, /approve, /noapprove")
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

		switch line {
		case "/exit", "/quit":
			return nil

		case "/approve":
			r.gate.AutoApprove = true
			fmt.Fprintln(out, "auto-approve enabled")
			continue

		case "/noapprove":
			r.gate.AutoApprove = false
			fmt.Fprintln(out, "auto-approve disabled")
			continue

		case "/tools":
			for _, t := range r.agent.Tools() {
				spec := t.Spec()
				fmt.Fprintf(out, "- %s: %s\n", spec.Name, spec.Description)
			}
			continue

		case "/skills":
			skills, err := LoadSkills(ctx, r.cfg.SkillsDir)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				continue
			}
			for _, s := range skills {
				fmt.Fprintf(out, "- %s (%s)\n", s.Name, s.Path)
			}
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
		resp, err := r.agent.Generate(reqCtx, r.cfg.SessionID, line)
		cancel()

		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}

		fmt.Fprintf(out, "\n%s\n\n", strings.TrimSpace(resp.(string)))
	}
}

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

	fmt.Fprintln(out, strings.TrimSpace(resp.(string)))
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
