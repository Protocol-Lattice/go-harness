package harness

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
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
	model       models.Agent
	gate        *ApprovalGate
	memoryStore *memory.MarkdownStore
}

const memoryFlushTimeout = 10 * time.Second

var (
	numberedTaskLinePattern = regexp.MustCompile(`^\s*\d+[\.)]\s+(.+?)\s*$`)
	requestedTaskCountRE    = regexp.MustCompile(`(?i)\bin\s+(\d+)\s+tasks?\b`)
)

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
	gate := &ApprovalGate{
		AutoApprove: cfg.AutoApprove,
		In:          stdin,
		Out:         stdout,
	}
	approvedClient := NewApprovingUTCPClient(client, gate, DefaultToolApprovalPolicy())
	systemPrompt := BuildSystemPrompt(skills, cfg.Workspace)

	a, err := agent.New(agent.Options{
		Model:        model,
		Memory:       mem,
		SystemPrompt: systemPrompt,
		UTCPClient:   approvedClient,
		CodeMode:     codemode.NewCodeModeUTCP(approvedClient, model),

		AllowUnsafeTools: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return &Runtime{
		cfg:         cfg,
		agent:       a,
		model:       model,
		memoryStore: mdStore,
		gate:        gate,
	}, nil
}

func (r *Runtime) RunREPL(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	restoreApprovalInput := r.bindApprovalScanner(scanner)
	defer restoreApprovalInput()

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

func (r *Runtime) RunTaskLoop(ctx context.Context, in io.Reader, out io.Writer) error {
	loop := taskLoop{
		plan:        r.planTasks,
		run:         r.RunOnce,
		slash:       r.runSlashCommand,
		printHelp:   r.printTaskLoopHelp,
		bindScanner: r.bindApprovalScanner,
	}
	return loop.Run(ctx, in, out)
}

func (r *Runtime) bindApprovalScanner(scanner *bufio.Scanner) func() {
	if r.gate == nil {
		return func() {}
	}

	previous := r.gate.readLine
	r.gate.readLine = func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	return func() {
		r.gate.readLine = previous
	}
}

func (r *Runtime) planTasks(ctx context.Context, task string) ([]string, error) {
	if r.model == nil {
		return nil, errors.New("task planner model is not configured")
	}

	reqCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	limit := plannedTaskLimit(task, r.cfg)
	resp, err := r.model.Generate(reqCtx, buildTaskPlanPrompt(task, limit))
	if err != nil {
		return nil, err
	}
	tasks := filterExecutableTasks(parseNumberedTasks(fmt.Sprint(resp)))
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return tasks, nil
}

func buildTaskPlanPrompt(task string, maxTasks int) string {
	if maxTasks < 2 {
		maxTasks = 2
	}

	taskCountInstruction := fmt.Sprintf("Break this task into 2-%d numbered subtasks.", maxTasks)
	if requested, ok := requestedTaskCount(task); ok {
		taskCountInstruction = fmt.Sprintf("Break this task into exactly %d numbered subtasks.", clampPlannedTaskCount(requested, maxTasks))
	}

	return fmt.Sprintf(`%s

Original task:
%s

Rules:
- Every subtask must include enough context to execute without reading other subtasks.
- If the task asks for a file, include the file path and content intent in the same subtask.
- If the user says "with the following content" but provides no content, infer a minimal useful implementation for the requested project.
- For Go MCP server requests, file-writing subtasks must include intent to create a complete compilable main.go.
- Shell commands must remain shell-command subtasks, not Go code-generation subtasks.
- Do not create a separate "run executable" subtask for long-running servers unless the task explicitly asks to verify startup briefly.
- Do not create empty placeholder files; file-writing subtasks must request complete useful content.
- Do not create navigation-only subtasks such as "cd", "navigate into", or "enter directory"; use paths instead.
- Do not ask follow-up questions; choose safe defaults when details are missing.
- Return only numbered lines.
- Each line must be one concrete action.
- Do not use markdown fences.
- Do not use nested bullets.
- Do not include extra prose.`, taskCountInstruction, strings.TrimSpace(task))
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

func plannedTaskLimit(task string, cfg Config) int {
	maxTasks := maxPlannedTasks(cfg)
	if requested, ok := requestedTaskCount(task); ok {
		return clampPlannedTaskCount(requested, maxTasks)
	}
	return maxTasks
}

func requestedTaskCount(task string) (int, bool) {
	match := requestedTaskCountRE.FindStringSubmatch(task)
	if len(match) != 2 {
		return 0, false
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return count, true
}

func clampPlannedTaskCount(count int, maxTasks int) int {
	if maxTasks < 2 {
		maxTasks = 2
	}
	if count < 2 {
		return 2
	}
	if count > maxTasks {
		return maxTasks
	}
	return count
}

func filterExecutableTasks(tasks []string) []string {
	filtered := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if isNavigationOnlyTask(task) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered
}

func isNavigationOnlyTask(task string) bool {
	lower := strings.ToLower(strings.TrimSpace(task))
	lower = strings.Trim(lower, ".")
	switch {
	case strings.HasPrefix(lower, "navigate into "):
		return true
	case strings.HasPrefix(lower, "change into "):
		return true
	case strings.HasPrefix(lower, "enter "):
		return true
	case strings.HasPrefix(lower, "cd "):
		return true
	default:
		return false
	}
}

type taskLoop struct {
	plan        func(context.Context, string) ([]string, error)
	run         func(context.Context, string, io.Writer) error
	slash       func(context.Context, string, io.Writer) error
	printHelp   func(io.Writer)
	bindScanner func(*bufio.Scanner) func()
}

func (l taskLoop) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	if l.bindScanner != nil {
		restoreApprovalInput := l.bindScanner(scanner)
		defer restoreApprovalInput()
	}

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

		runTasks(ctx, line, tasks, out, l.run)
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
	originalTask string,
	tasks []string,
	out io.Writer,
	run func(context.Context, string, io.Writer) error,
) {
	for i, task := range tasks {
		fmt.Fprintf(out, "[%d/%d] %s\n", i+1, len(tasks), task)
		if err := run(ctx, buildTaskExecutionPrompt(originalTask, tasks, i), out); err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			return
		}
	}
}

func buildTaskExecutionPrompt(originalTask string, tasks []string, index int) string {
	var b strings.Builder

	b.WriteString("Execute exactly one approved subtask from a task-loop plan.\n\n")

	b.WriteString("Original task:\n")
	b.WriteString(strings.TrimSpace(originalTask))
	b.WriteString("\n\n")

	b.WriteString("Full approved task list:\n")
	for i, task := range tasks {
		fmt.Fprintf(&b, "%d. %s\n", i+1, task)
	}

	b.WriteString("\nCurrent subtask:\n")
	if index >= 0 && index < len(tasks) {
		b.WriteString(strings.TrimSpace(tasks[index]))
	}
	b.WriteString("\n\n")

	b.WriteString("Execution rules:\n")
	b.WriteString("- Execute only the current subtask.\n")
	b.WriteString("- Use filesystem tools for directory and file operations.\n")
	b.WriteString("- Use shell tools for shell commands such as go mod init, go build, go run, gofmt, and executable runs.\n")
	b.WriteString("- Do not use CodeMode for filesystem or shell tasks.\n")
	b.WriteString("- Do not generate Go snippets for shell commands.\n")
	b.WriteString("- Do not emit package declarations, import blocks, or func main snippets unless writing them into a file.\n")
	b.WriteString("- If writing a file, write the complete intended file content using the filesystem tool.\n")
	b.WriteString("- Do not create empty placeholder files.\n")
	b.WriteString("- Do not ask follow-up questions; choose safe defaults when details are missing.\n")
	b.WriteString("- After tool execution, return a short factual result based only on tool output.\n")

	return b.String()
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
		if r.gate == nil {
			r.gate = &ApprovalGate{Out: out}
		}
		r.gate.AutoApprove = true
		fmt.Fprintln(out, "auto-approve enabled")
		return nil

	case "/noapprove", "/no-approve":
		if r.gate == nil {
			r.gate = &ApprovalGate{Out: out}
		}
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

func (r *Runtime) printTaskLoopHelp(out io.Writer) {
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
