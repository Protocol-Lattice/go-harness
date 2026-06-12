package harness

import (
	"context"
	"errors"
	"strings"
	"testing"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

func TestToolApprovalAllowsReadOnlyToolWithoutPrompt(t *testing.T) {
	inner := &stubApprovalUTCPClient{}
	var out strings.Builder
	gate := &ApprovalGate{
		In:  strings.NewReader("n\n"),
		Out: &out,
	}

	client := NewApprovingUTCPClient(inner, gate, DefaultToolApprovalPolicy())

	result, err := client.CallTool(context.Background(), "filesystem.read", map[string]any{
		"path": "README.md",
	})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if result != "called filesystem.read" {
		t.Fatalf("result = %v, want read result", result)
	}
	if inner.callCount != 1 {
		t.Fatalf("inner call count = %d, want 1", inner.callCount)
	}
	if out.String() != "" {
		t.Fatalf("approval prompt = %q, want empty", out.String())
	}
}

func TestToolApprovalDeniesMutatingToolWhenUserRejects(t *testing.T) {
	inner := &stubApprovalUTCPClient{}
	var out strings.Builder
	gate := &ApprovalGate{
		In:  strings.NewReader("n\n"),
		Out: &out,
	}

	client := NewApprovingUTCPClient(inner, gate, DefaultToolApprovalPolicy())

	_, err := client.CallTool(context.Background(), "filesystem.write", map[string]any{
		"path":    "file.txt",
		"content": "hello",
	})
	if err == nil {
		t.Fatal("CallTool returned nil error, want denial")
	}
	if !strings.Contains(err.Error(), "tool call denied: filesystem.write") {
		t.Fatalf("error = %q, want denial", err.Error())
	}
	if inner.callCount != 0 {
		t.Fatalf("inner call count = %d, want 0", inner.callCount)
	}
	prompt := out.String()
	if !strings.Contains(prompt, "filesystem.write") {
		t.Fatalf("prompt = %q, want tool name", prompt)
	}
	if !strings.Contains(prompt, `"path":"file.txt"`) {
		t.Fatalf("prompt = %q, want compact args", prompt)
	}
}

func TestToolApprovalAutoApproveBypassesPrompt(t *testing.T) {
	inner := &stubApprovalUTCPClient{}
	var out strings.Builder
	gate := &ApprovalGate{
		AutoApprove: true,
		Out:         &out,
	}

	client := NewApprovingUTCPClient(inner, gate, DefaultToolApprovalPolicy())

	_, err := client.CallTool(context.Background(), "shell.run", map[string]any{
		"argv": []any{"go", "test", "./..."},
	})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if inner.callCount != 1 {
		t.Fatalf("inner call count = %d, want 1", inner.callCount)
	}
	if out.String() != "" {
		t.Fatalf("approval prompt = %q, want empty", out.String())
	}
}

func TestToolApprovalRequiresApprovalForStreamCalls(t *testing.T) {
	inner := &stubApprovalUTCPClient{}
	var out strings.Builder
	gate := &ApprovalGate{
		In:  strings.NewReader("n\n"),
		Out: &out,
	}

	client := NewApprovingUTCPClient(inner, gate, DefaultToolApprovalPolicy())

	_, err := client.CallToolStream(context.Background(), "git.diff", nil)
	if err == nil {
		t.Fatal("CallToolStream returned nil error, want denial")
	}
	if !strings.Contains(err.Error(), "tool call denied: git.diff") {
		t.Fatalf("error = %q, want denial", err.Error())
	}
	if inner.streamCount != 0 {
		t.Fatalf("inner stream count = %d, want 0", inner.streamCount)
	}
	if !strings.Contains(out.String(), "git.diff") {
		t.Fatalf("prompt = %q, want tool name", out.String())
	}
}

type stubApprovalUTCPClient struct {
	callCount   int
	streamCount int
}

var _ utcp.UtcpClientInterface = (*stubApprovalUTCPClient)(nil)

func (s *stubApprovalUTCPClient) RegisterToolProvider(
	context.Context,
	base.Provider,
) ([]tools.Tool, error) {
	return nil, nil
}

func (s *stubApprovalUTCPClient) DeregisterToolProvider(context.Context, string) error {
	return nil
}

func (s *stubApprovalUTCPClient) CallTool(
	_ context.Context,
	toolName string,
	_ map[string]any,
) (any, error) {
	s.callCount++
	return "called " + toolName, nil
}

func (s *stubApprovalUTCPClient) SearchTools(string, int) ([]tools.Tool, error) {
	return nil, nil
}

func (s *stubApprovalUTCPClient) GetTransports() map[string]repository.ClientTransport {
	return nil
}

func (s *stubApprovalUTCPClient) CallToolStream(
	context.Context,
	string,
	map[string]any,
) (transports.StreamResult, error) {
	s.streamCount++
	return nil, errors.New("stream not implemented")
}
