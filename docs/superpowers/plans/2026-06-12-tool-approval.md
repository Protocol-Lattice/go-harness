# Tool Approval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add provider-level approval checks for go-harness tool calls.

**Architecture:** Wrap the existing `utcp.UtcpClientInterface` in `internal/harness`. The wrapper delegates non-execution methods unchanged and gates `CallTool` / `CallToolStream` through `ApprovalGate` using a small name-based policy.

**Tech Stack:** Go 1.26.3, standard library, existing `go-agent` and `go-utcp` interfaces.

---

### Task 1: Approval Policy and Wrapper

**Files:**
- Create: `internal/harness/tool_approval.go`
- Test: `internal/harness/tool_approval_test.go`
- Modify: `internal/harness/runtime.go`

- [ ] **Step 1: Write failing wrapper tests**

Create tests for allow, deny, auto-approve, and stream approval using a stub
`utcp.UtcpClientInterface`.

- [ ] **Step 2: Verify tests fail**

Run:

```sh
rtk go test ./internal/harness -run 'TestToolApproval'
```

Expected: compile failure for missing approval wrapper types/functions.

- [ ] **Step 3: Implement wrapper**

Create `ToolApprovalPolicy`, `approvingUTCPClient`, and a constructor used by
`NewRuntime`.

- [ ] **Step 4: Wire runtime**

Build one `ApprovalGate` pointer in `NewRuntime`, wrap the UTCP client, pass the
wrapped client to agent and CodeMode, and update slash commands to mutate the
same gate pointer.

- [ ] **Step 5: Verify**

Run:

```sh
rtk gofmt -w internal/harness
rtk go test ./internal/harness
rtk go test ./...
```
