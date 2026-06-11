# go-harness CodeMode normalization fix

Replace your harness files with the files in `internal/harness/`.

This patch targets the repeated `error: snippet validation failed` cases where the model emits invalid CodeMode snippets such as:

- JSON envelopes containing `{ "code": "..." }`
- `return __out`
- `_, err := codemode.CallTool(...)`
- `runResult` / `runRes` result aliases
- `shell.run` called with `[]string{...}` instead of `map[string]any{"argv": ...}`
- invented `codemode.Errorf` and `codemode.Sprintf`
- missing trailing commas in multiline `map[string]any{...}` literals

## Apply

```bash
cp internal/harness/codemode_normalize.go /path/to/go-harness/internal/harness/codemode_normalize.go
cp internal/harness/codemode_normalize_test.go /path/to/go-harness/internal/harness/codemode_normalize_test.go
go test ./...
```

## Important

If your runtime still logs `Skipping invalid snippet:` with the raw bad snippet, then the normalizer is still not being called at the final CodeMode validation boundary.

In that case, find the validator/log site:

```bash
grep -R "Skipping invalid snippet" -n .
```

Then call `NormalizeCodeModeSnippet(snippet)` immediately before validation/execution.
