package harness

import (
	"context"
	"encoding/json"
	"fmt"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

type ToolApprovalPolicy struct {
	allowWithoutPrompt map[string]struct{}
}

func DefaultToolApprovalPolicy() ToolApprovalPolicy {
	allow := map[string]struct{}{
		"filesystem.read":   {},
		"filesystem.list":   {},
		"filesystem.stat":   {},
		"filesystem.exists": {},
		"git.status":        {},
		"git.diff":          {},
		"git.log":           {},
	}
	return ToolApprovalPolicy{allowWithoutPrompt: allow}
}

func (p ToolApprovalPolicy) RequiresApproval(toolName string, stream bool) bool {
	if stream {
		return true
	}
	if p.allowWithoutPrompt == nil {
		p = DefaultToolApprovalPolicy()
	}
	_, ok := p.allowWithoutPrompt[toolName]
	return !ok
}

func NewApprovingUTCPClient(
	inner utcp.UtcpClientInterface,
	gate *ApprovalGate,
	policy ToolApprovalPolicy,
) utcp.UtcpClientInterface {
	if inner == nil {
		return nil
	}
	return &approvingUTCPClient{
		inner:  inner,
		gate:   gate,
		policy: policy,
	}
}

type approvingUTCPClient struct {
	inner  utcp.UtcpClientInterface
	gate   *ApprovalGate
	policy ToolApprovalPolicy
}

func (c *approvingUTCPClient) RegisterToolProvider(
	ctx context.Context,
	prov base.Provider,
) ([]tools.Tool, error) {
	return c.inner.RegisterToolProvider(ctx, prov)
}

func (c *approvingUTCPClient) DeregisterToolProvider(
	ctx context.Context,
	providerName string,
) error {
	return c.inner.DeregisterToolProvider(ctx, providerName)
}

func (c *approvingUTCPClient) CallTool(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (any, error) {
	if err := c.approve(toolName, args, false); err != nil {
		return nil, err
	}
	return c.inner.CallTool(ctx, toolName, args)
}

func (c *approvingUTCPClient) SearchTools(query string, limit int) ([]tools.Tool, error) {
	return c.inner.SearchTools(query, limit)
}

func (c *approvingUTCPClient) GetTransports() map[string]repository.ClientTransport {
	return c.inner.GetTransports()
}

func (c *approvingUTCPClient) CallToolStream(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (transports.StreamResult, error) {
	if err := c.approve(toolName, args, true); err != nil {
		return nil, err
	}
	return c.inner.CallToolStream(ctx, toolName, args)
}

func (c *approvingUTCPClient) approve(toolName string, args map[string]any, stream bool) error {
	if !c.policy.RequiresApproval(toolName, stream) {
		return nil
	}
	if c.gate != nil && c.gate.Approve(approvalReason(toolName, args, stream)) {
		return nil
	}
	return fmt.Errorf("tool call denied: %s", toolName)
}

func approvalReason(toolName string, args map[string]any, stream bool) string {
	kind := "tool"
	if stream {
		kind = "stream tool"
	}
	return fmt.Sprintf("%s %s args=%s", kind, toolName, compactToolArgs(args))
}

func compactToolArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("%v", args)
	}
	return string(data)
}
