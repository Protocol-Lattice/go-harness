package harness

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	agent "github.com/Protocol-Lattice/go-agent"
	"github.com/Protocol-Lattice/go-agent/src/memory"
	"github.com/Protocol-Lattice/go-agent/src/models"
	"github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/plugins/codemode"
)

type Runtime struct {
	cfg         Config
	agent       *agent.Agent
	gate        ApprovalGate
	memoryStore *memory.MarkdownStore
}

const memoryFlushTimeout = 10 * time.Second

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
		cfg:         cfg,
		agent:       a,
		memoryStore: mdStore,
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
	store, recordsBefore, err := r.sessionRecordSnapshot(ctx)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	resp, err := r.agent.Generate(reqCtx, r.cfg.SessionID, prompt)
	if err != nil {
		return err
	}

	response := strings.TrimSpace(fmt.Sprint(resp))
	fmt.Fprintln(out, response)

	flushCtx, flushCancel := context.WithTimeout(ctx, memoryFlushTimeout)
	defer flushCancel()
	if err := r.agent.Flush(flushCtx, r.cfg.SessionID); err != nil {
		return fmt.Errorf("save memory: %w", err)
	}
	if err := r.saveTranscriptIfMissing(flushCtx, store, recordsBefore, prompt, response); err != nil {
		return fmt.Errorf("save transcript fallback: %w", err)
	}

	return nil
}

func (r *Runtime) sessionRecordSnapshot(ctx context.Context) (*memory.MarkdownStore, int, error) {
	store, err := r.transcriptStore()
	if err != nil {
		return nil, 0, err
	}
	count, err := countSessionRecords(ctx, store, r.cfg.SessionID)
	return store, count, err
}

func (r *Runtime) transcriptStore() (*memory.MarkdownStore, error) {
	if r.memoryStore != nil {
		return r.memoryStore, nil
	}
	if strings.TrimSpace(r.cfg.MemoryDir) == "" {
		return nil, nil
	}
	return memory.NewMarkdownStore(r.cfg.MemoryDir)
}

func countSessionRecords(ctx context.Context, store *memory.MarkdownStore, sessionID string) (int, error) {
	if store == nil {
		return 0, nil
	}
	records, err := store.List(ctx, "sessions", sessionID)
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

func (r *Runtime) saveTranscriptIfMissing(
	ctx context.Context,
	store *memory.MarkdownStore,
	recordsBefore int,
	prompt string,
	response string,
) error {
	if store == nil {
		return nil
	}

	recordsAfter, err := countSessionRecords(ctx, store, r.cfg.SessionID)
	if err != nil {
		return err
	}
	if recordsAfter > recordsBefore {
		return nil
	}

	metadata := map[string]any{"source": "runtime_fallback"}
	if strings.TrimSpace(prompt) != "" {
		if err := store.Save(ctx, memory.MarkdownRecord{
			Scope:     "sessions",
			SessionID: r.cfg.SessionID,
			Role:      "user",
			Content:   prompt,
			Metadata:  metadata,
		}); err != nil {
			return err
		}
	}
	if strings.TrimSpace(response) != "" {
		if err := store.Save(ctx, memory.MarkdownRecord{
			Scope:     "sessions",
			SessionID: r.cfg.SessionID,
			Role:      "assistant",
			Content:   response,
			Metadata:  metadata,
		}); err != nil {
			return err
		}
	}
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
