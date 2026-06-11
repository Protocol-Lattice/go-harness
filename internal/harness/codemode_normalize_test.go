package harness

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeCodeModeSnippetRepairsGeminiToolSnippet(t *testing.T) {
	input := `_, err := codemode.CallTool("filesystem.write", map[string]any{
    "path": "main.go",
    "content": "package main\\n",
})
if err != nil {
    __out = err
    return __out
}

runResult, err := codemode.CallTool("shell.run", map[string]any{
    "argv": []string{"go", "run", "main.go"},
})
if err != nil {
    __out = err
    return __out
}

__out = runResult`

	got := NormalizeCodeModeSnippet(input)

	mustContain := []string{
		"__out = func() any {",
		"result, err := codemode.CallTool",
		"result, err = codemode.CallTool",
		"return err",
		"return result",
		"}()",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized snippet missing %q:\n%s", want, got)
		}
	}

	mustNotContain := []string{
		"_, err :=",
		"runResult",
		"return __out",
		"__out = err",
		"__out = result",
	}
	for _, bad := range mustNotContain {
		if strings.Contains(got, bad) {
			t.Fatalf("normalized snippet still contains %q:\n%s", bad, got)
		}
	}
}

func TestNormalizeCodeModeSnippetExtractsJSONCodeAndRepairsShellRunArgv(t *testing.T) {
	input := "```json\n{\n  \"needs\": true\n}\n```\n" +
		"```json\n{\n  \"tools\": [\"filesystem.write\", \"shell.run\"]\n}\n```\n" +
		"```json\n{\n  \"code\": \"var err error\\n\\n_, err = codemode.CallTool(\\\"filesystem.write\\\", map[string]any{\\n    \\\"path\\\": \\\"main.go\\\",\\n    \\\"content\\\": \\\"package main\\\\n\\\",\\n})\\nif err != nil {\\n    __out = err\\n    return __out\\n}\\n\\nrunResult, err := codemode.CallTool(\\\"shell.run\\\", []string{\\\"go\\\", \\\"run\\\", \\\"main.go\\\"})\\nif err != nil {\\n    __out = err\\n    return __out\\n}\\n\\n__out = runResult\",\n  \"stream\": false\n}\n```"

	got := NormalizeCodeModeSnippet(input)

	mustContain := []string{
		"__out = func() any {",
		"result, err := codemode.CallTool(\"filesystem.write\"",
		"result, err = codemode.CallTool(\"shell.run\", map[string]any{\"argv\": []string{\"go\", \"run\", \"main.go\"}})",
		"return err",
		"return result",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized snippet missing %q:\n%s", want, got)
		}
	}

	mustNotContain := []string{
		"```json",
		"\"needs\"",
		"[]string{\"go\", \"run\", \"main.go\"})",
		"return __out",
		"runResult",
		"__out = err",
	}
	for _, bad := range mustNotContain {
		if strings.Contains(got, bad) {
			t.Fatalf("normalized snippet still contains %q:\n%s", bad, got)
		}
	}
}

func TestNormalizeModelOutputRepairsNestedCodeModeRunCodeArgument(t *testing.T) {
	code := `_, err := codemode.CallTool("filesystem.write", map[string]any{
    "path": "main.go",
    "content": "package main\n"
})
if err != nil {
    __out = err
    return __out
}

runResult, err := codemode.CallTool("shell.run", []string{"go", "run", "main.go"})
if err != nil {
    __out = err
    return __out
}

__out = runResult`

	inputBytes, err := json.Marshal(map[string]any{
		"use_tool":  true,
		"tool_name": "codemode.run_code",
		"arguments": map[string]any{
			"code":    code,
			"timeout": 20000,
		},
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	got, ok := normalizeModelOutput(string(inputBytes)).(string)
	if !ok {
		t.Fatalf("expected normalized model output to remain a string")
	}

	var payload struct {
		UseTool   bool           `json:"use_tool"`
		ToolName  string         `json:"tool_name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("normalized output is not valid JSON: %v\n%s", err, got)
	}

	if !payload.UseTool || payload.ToolName != "codemode.run_code" {
		t.Fatalf("tool choice fields changed: %+v", payload)
	}

	normalizedCode, ok := payload.Arguments["code"].(string)
	if !ok {
		t.Fatalf("expected arguments.code string, got %#v", payload.Arguments["code"])
	}

	mustContain := []string{
		"__out = func() any {",
		`"content": "package main\n",`,
		`result, err := codemode.CallTool("filesystem.write"`,
		`result, err = codemode.CallTool("shell.run", map[string]any{"argv": []string{"go", "run", "main.go"}})`,
		"return err",
		"return result",
	}
	for _, want := range mustContain {
		if !strings.Contains(normalizedCode, want) {
			t.Fatalf("normalized nested snippet missing %q:\n%s", want, normalizedCode)
		}
	}

	mustNotContain := []string{
		"_, err :=",
		"runResult",
		"return __out",
		`"content": "package main\n"` + "\n}",
	}
	for _, bad := range mustNotContain {
		if strings.Contains(normalizedCode, bad) {
			t.Fatalf("normalized nested snippet still contains %q:\n%s", bad, normalizedCode)
		}
	}
}

func TestNormalizeCodeModeSnippetLeavesProseAlone(t *testing.T) {
	input := "I cannot use tools because no filesystem schema is available."
	if got := NormalizeCodeModeSnippet(input); got != input {
		t.Fatalf("expected prose unchanged, got %q", got)
	}
}

func TestNormalizeModelOutputRepairsCodeModeGoProgramJSON(t *testing.T) {
	input := `{
  "code": "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}",
  "stream": false
}`

	got, ok := normalizeModelOutputForPrompt(input, codeModeGenerationPromptForTest("create simple hello world application golang in new folder")).(string)
	if !ok {
		t.Fatalf("expected string output")
	}

	var payload struct {
		Code   string `json:"code"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("normalized output is not valid JSON: %v\n%s", err, got)
	}

	mustContain := []string{
		`__out = func() any {`,
		`codemode.CallTool("filesystem.mkdir"`,
		`"path": "hello-world-go"`,
		`codemode.CallTool("filesystem.write"`,
		`"path": "hello-world-go/main.go"`,
		`package main\n\nfunc main()`,
		`return map[string]any{`,
	}
	for _, want := range mustContain {
		if !strings.Contains(payload.Code, want) {
			t.Fatalf("normalized code missing %q:\n%s", want, payload.Code)
		}
	}

	if strings.HasPrefix(strings.TrimSpace(payload.Code), "package main") {
		t.Fatalf("normalized code still starts with package declaration:\n%s", payload.Code)
	}
	if payload.Stream {
		t.Fatalf("stream flag changed to true")
	}
}

func TestNormalizeModelOutputRepairsBareGoProgramAfterMkdir(t *testing.T) {
	input := `{
  "code": "mkdirResult, err := codemode.CallTool(\"filesystem.mkdir\", map[string]any{\"path\":\"hello-world-go\"})\nif err != nil {\n    __out = err\n    return\n}\n\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n\n__out = mkdirResult",
  "stream": false
}`

	got, ok := normalizeModelOutputForPrompt(input, codeModeGenerationPromptForTest("create new folder and in that folder create simple hello world application golang")).(string)
	if !ok {
		t.Fatalf("expected string output")
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("normalized output is not valid JSON: %v\n%s", err, got)
	}

	mustContain := []string{
		`codemode.CallTool("filesystem.mkdir"`,
		`codemode.CallTool("filesystem.write"`,
		`"path": "hello-world-go/main.go"`,
		`fmt.Println(\"Hello, World!\")`,
	}
	for _, want := range mustContain {
		if !strings.Contains(payload.Code, want) {
			t.Fatalf("normalized code missing %q:\n%s", want, payload.Code)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(payload.Code), "package main") {
		t.Fatalf("normalized code still starts with package declaration:\n%s", payload.Code)
	}
}

func TestNormalizeModelOutputRepairsInlineGoProgramAssignment(t *testing.T) {
	input := `{
  "code": "mkdirResult, err := codemode.CallTool(\"filesystem.mkdir\", map[string]any{\"path\":\"hello-world-go\"})\nif err != nil {\n    __out = err\n    return\n}\n\ncontent := package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n\nwriteResult, err := codemode.CallTool(\"filesystem.write\", map[string]any{\"path\":\"hello-world-go/main.go\", \"content\": content})\nif err != nil {\n    __out = err\n    return\n}\n\n__out = writeResult",
  "stream": false
}`

	got, ok := normalizeModelOutputForPrompt(input, codeModeGenerationPromptForTest("create simple hello world application in golang in new folder")).(string)
	if !ok {
		t.Fatalf("expected string output")
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("normalized output is not valid JSON: %v\n%s", err, got)
	}

	mustContain := []string{
		`__out = func() any {`,
		`codemode.CallTool("filesystem.mkdir"`,
		`codemode.CallTool("filesystem.write"`,
		`"path": "hello-world-go/main.go"`,
		`fmt.Println(\"Hello, World!\")`,
	}
	for _, want := range mustContain {
		if !strings.Contains(payload.Code, want) {
			t.Fatalf("normalized code missing %q:\n%s", want, payload.Code)
		}
	}

	if strings.Contains(payload.Code, "content := package main") {
		t.Fatalf("normalized code still contains invalid inline package assignment:\n%s", payload.Code)
	}
}

func codeModeGenerationPromptForTest(query string) string {
	return "Generate a Go snippet that uses ONLY the following UTCP tools:\n\nUSER QUERY:\n" +
		strconv.Quote(query) +
		"\n\nTOOL SPECS:\nfilesystem.mkdir filesystem.write shell.run\n\nRespond ONLY in JSON:\n{\n  \"code\": \"<go snippet>\",\n  \"stream\": false\n}"
}

func TestNormalizeCodeModeSnippetRepairsBuildAndRunSnippet(t *testing.T) {
	input := `var err error

// 1. Write the Go "Hello, World!" code to a file named main.go
_, err = codemode.CallTool("filesystem.write", map[string]any{
    "path": "main.go",
    "content": "package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}"
})
if err != nil {
    __out = err
    return __out
}

// 2. Compile the Go application
compileResult, err := codemode.CallTool("shell.run", map[string]any{
    "argv": []string{"go", "build", "main.go"}
})
if err != nil {
    __out = err
    return __out
}

// Check for compilation errors (stdout/stderr might contain messages)
if m, ok := compileResult.(map[string]any); ok {
    if stderr, hasStderr := m["stderr"]; hasStderr && stderr != "" {
        __out = codemode.Errorf("Compilation failed: %s", stderr)
        return __out
    }
}

// 3. Run the compiled application
runResult, err := codemode.CallTool("shell.run", map[string]any{
    "argv": []string{"." + codemode.Sprintf("/main")}
})
if err != nil {
    __out = err
    return __out
}

__out = map[string]any{
    "compilation_output": compileResult,
    "execution_output": runResult,
}`

	got := NormalizeCodeModeSnippet(input)

	mustContain := []string{
		`__out = func() any {`,
		`"content": "package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}",`,
		`return err`,
		`map[string]any{"error": "Compilation failed", "stderr": stderr}`,
		`"argv": []string{"./main"}`,
		`return map[string]any{`,
		`}()`,
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized snippet missing %q:\n%s", want, got)
		}
	}

	mustNotContain := []string{
		`return __out`,
		`codemode.Errorf`,
		`codemode.Sprintf`,
		`__out = err`,
		`__out = map[string]any`,
		`runResult`,
	}
	for _, bad := range mustNotContain {
		if strings.Contains(got, bad) {
			t.Fatalf("normalized snippet still contains %q:\n%s", bad, got)
		}
	}
}
