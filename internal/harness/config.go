package harness

import (
	"flag"
	"os"
	"time"
)

type Config struct {
	Provider      string
	Model         string
	SessionID     string
	SkillsDir     string
	ProvidersFile string
	Workspace     string
	MaxTurns      int
	Timeout       time.Duration
	AutoApprove   bool
	GitHubToken   string
	MemoryDir     string
}

func ParseFlags() Config {
	var cfg Config

	flag.StringVar(&cfg.Provider, "provider", "openai", "LLM provider: openai, gemini, anthropic, ollama")
	flag.StringVar(&cfg.Model, "model", "gpt-4o-mini", "model name")
	flag.StringVar(&cfg.SessionID, "session", "agentic-tui", "memory session id")
	flag.StringVar(&cfg.SkillsDir, "skills", "./skills", "directory containing markdown skills")
	flag.StringVar(&cfg.ProvidersFile, "providers", "./providers.json", "UTCP providers file")
	flag.StringVar(&cfg.Workspace, "workspace", ".", "workspace root for filesystem tools")
	flag.IntVar(&cfg.MaxTurns, "max-turns", 8, "max autonomous loop turns")
	flag.DurationVar(&cfg.Timeout, "timeout", 2*time.Minute, "per request timeout")
	flag.BoolVar(&cfg.AutoApprove, "y", false, "auto-approve tool/code execution")
	flag.StringVar(&cfg.GitHubToken, "github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token for private repos or higher rate limits")
	flag.StringVar(&cfg.MemoryDir, "memory", ".agent-memory", "markdown memory directory")
	flag.Parse()
	return cfg
}
