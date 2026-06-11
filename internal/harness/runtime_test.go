package harness

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agent "github.com/Protocol-Lattice/go-agent"
	"github.com/Protocol-Lattice/go-agent/src/memory"
	"github.com/Protocol-Lattice/go-agent/src/models"
)

func TestRunOnceFlushesMemoryToMarkdownStore(t *testing.T) {
	ctx := context.Background()
	memoryDir := t.TempDir()

	store, err := memory.NewMarkdownStore(memoryDir)
	if err != nil {
		t.Fatalf("create markdown store: %v", err)
	}

	mem := memory.NewSessionMemory(memory.NewMemoryBankWithStore(store), 16).
		WithEmbedder(memory.DummyEmbedder{})

	a, err := agent.New(agent.Options{
		Model:        models.NewDummyLLM("assistant:"),
		Memory:       mem,
		SystemPrompt: "You are a test assistant.",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rt := &Runtime{
		cfg: Config{
			SessionID: "memory-test",
			Timeout:   time.Second,
		},
		agent: a,
	}

	var out bytes.Buffer
	if err := rt.RunOnce(ctx, "remember blue", &out); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	path := filepath.Join(memoryDir, "sessions", "memory-test.md")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted memory file: %v", err)
	}

	text := string(contents)
	if !strings.Contains(text, "remember blue") {
		t.Fatalf("persisted memory missing user turn:\n%s", text)
	}
	if !strings.Contains(text, "assistant:") {
		t.Fatalf("persisted memory missing assistant turn:\n%s", text)
	}
}
