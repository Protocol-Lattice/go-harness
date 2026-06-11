package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Inputs      map[string]any `json:"inputs"`
}

type Result struct {
	OK    bool   `json:"ok"`
	Tool  string `json:"tool"`
	Path  string `json:"path,omitempty"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

type pathInput struct {
	Path string `json:"path"`
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Append  bool   `json:"append,omitempty"`
}

type renameInput struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

type copyInput struct {
	SrcPath string `json:"src_path"`
	DstPath string `json:"dst_path"`
}

func main() {
	root := flag.String("root", ".", "filesystem root")
	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		writeErr("", "", err)
		os.Exit(2)
	}

	args := flag.Args()
	if len(args) == 0 {
		writeErr("", "", errors.New("missing command"))
		os.Exit(2)
	}

	if args[0] == "list-tools" {
		writeJSON(tools())
		return
	}

	if len(args) < 2 {
		writeErr(args[0], "", errors.New("missing json input argument"))
		os.Exit(2)
	}

	tool := normalizeTool(args[0])
	switch tool {
	case "filesystem.list":
		var in pathInput
		mustDecode(args[1], &in)
		runList(absRoot, args[0], in.Path)
	case "filesystem.read":
		var in pathInput
		mustDecode(args[1], &in)
		runRead(absRoot, args[0], in.Path)
	case "filesystem.write":
		var in writeInput
		mustDecode(args[1], &in)
		runWrite(absRoot, args[0], in)
	case "filesystem.mkdir":
		var in pathInput
		mustDecode(args[1], &in)
		runMkdir(absRoot, args[0], in.Path)
	case "filesystem.remove":
		var in pathInput
		mustDecode(args[1], &in)
		runRemove(absRoot, args[0], in.Path)
	case "filesystem.stat":
		var in pathInput
		mustDecode(args[1], &in)
		runStat(absRoot, args[0], in.Path)
	case "filesystem.rename":
		var in renameInput
		mustDecode(args[1], &in)
		runRename(absRoot, args[0], in)
	case "filesystem.copy":
		var in copyInput
		mustDecode(args[1], &in)
		runCopy(absRoot, args[0], in)
	case "filesystem.exists":
		var in pathInput
		mustDecode(args[1], &in)
		runExists(absRoot, args[0], in.Path)
	default:
		writeErr(args[0], "", fmt.Errorf("unknown tool %q", args[0]))
		os.Exit(2)
	}
}

func tools() []Tool {
	return []Tool{
		{Name: "filesystem.list", Description: "List files and directories under the configured root.", Inputs: schema([]string{"path"}, field("path", "string", "Directory path relative to root, use . for root"))},
		{Name: "filesystem.read", Description: "Read a file under the configured root.", Inputs: schema([]string{"path"}, field("path", "string", "File path relative to root"))},
		{Name: "filesystem.write", Description: "Write a file under the configured root. Creates parent directories as needed.", Inputs: schema([]string{"path", "content"}, field("path", "string", "File path relative to root"), field("content", "string", "Content to write"), field("append", "boolean", "Append instead of overwrite"))},
		{Name: "filesystem.mkdir", Description: "Create a directory under the configured root.", Inputs: schema([]string{"path"}, field("path", "string", "Directory path relative to root"))},
		{Name: "filesystem.remove", Description: "Remove a file or directory under the configured root.", Inputs: schema([]string{"path"}, field("path", "string", "File or directory path relative to root"))},
		{Name: "filesystem.stat", Description: "Get file or directory metadata under the configured root.", Inputs: schema([]string{"path"}, field("path", "string", "Path relative to root"))},
		{Name: "filesystem.rename", Description: "Rename or move a file or directory under the configured root.", Inputs: schema([]string{"old_path", "new_path"}, field("old_path", "string", "Existing path relative to root"), field("new_path", "string", "New path relative to root"))},
		{Name: "filesystem.copy", Description: "Copy a file under the configured root.", Inputs: schema([]string{"src_path", "dst_path"}, field("src_path", "string", "Source file path relative to root"), field("dst_path", "string", "Destination file path relative to root"))},
		{Name: "filesystem.exists", Description: "Check if a file or directory exists under the configured root.", Inputs: schema([]string{"path"}, field("path", "string", "Path relative to root"))},
	}
}

func normalizeTool(name string) string {
	if strings.HasPrefix(name, "fs.") {
		return "filesystem." + strings.TrimPrefix(name, "fs.")
	}
	return name
}

func field(name, typ, desc string) map[string]any {
	return map[string]any{name: map[string]any{"type": typ, "description": desc}}
}

func schema(required []string, fields ...map[string]any) map[string]any {
	props := map[string]any{}
	for _, f := range fields {
		for k, v := range f {
			props[k] = v
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func safePath(root, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("path is required")
	}
	if strings.ContainsRune(p, '\x00') {
		return "", errors.New("path contains NUL byte")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}

	abs, err := filepath.Abs(filepath.Join(root, p))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %s", p)
	}
	return abs, nil
}

func runList(root, tool, p string) {
	path, err := safePath(root, defaultPath(p))
	if err != nil {
		fail(tool, p, err)
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		fail(tool, p, err)
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		item := map[string]any{"name": e.Name(), "is_dir": e.IsDir()}
		if info != nil {
			item["size"] = info.Size()
		}
		out = append(out, item)
	}
	writeOK(tool, p, out)
}

func runRead(root, tool, p string) {
	path, err := safePath(root, p)
	if err != nil {
		fail(tool, p, err)
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		fail(tool, p, err)
		return
	}
	writeOK(tool, p, string(b))
}

func runWrite(root, tool string, in writeInput) {
	path, err := safePath(root, in.Path)
	if err != nil {
		fail(tool, in.Path, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(tool, in.Path, err)
		return
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if in.Append {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		fail(tool, in.Path, err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(in.Content); err != nil {
		fail(tool, in.Path, err)
		return
	}
	writeOK(tool, in.Path, map[string]any{"bytes": len(in.Content)})
}

func runMkdir(root, tool, p string) {
	path, err := safePath(root, p)
	if err != nil {
		fail(tool, p, err)
		return
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		fail(tool, p, err)
		return
	}
	writeOK(tool, p, map[string]any{"created": true})
}

func runRemove(root, tool, p string) {
	path, err := safePath(root, p)
	if err != nil {
		fail(tool, p, err)
		return
	}
	if err := os.RemoveAll(path); err != nil {
		fail(tool, p, err)
		return
	}
	writeOK(tool, p, map[string]any{"removed": true})
}

func runStat(root, tool, p string) {
	path, err := safePath(root, p)
	if err != nil {
		fail(tool, p, err)
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		fail(tool, p, err)
		return
	}
	writeOK(tool, p, map[string]any{"name": info.Name(), "size": info.Size(), "is_dir": info.IsDir(), "mode": info.Mode().String(), "modified": info.ModTime()})
}

func runRename(root, tool string, in renameInput) {
	oldPath, err := safePath(root, in.OldPath)
	if err != nil {
		fail(tool, in.OldPath, err)
		return
	}
	newPath, err := safePath(root, in.NewPath)
	if err != nil {
		fail(tool, in.NewPath, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		fail(tool, in.NewPath, err)
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		fail(tool, in.OldPath, err)
		return
	}
	writeOK(tool, in.NewPath, map[string]any{"renamed": true})
}

func runCopy(root, tool string, in copyInput) {
	src, err := safePath(root, in.SrcPath)
	if err != nil {
		fail(tool, in.SrcPath, err)
		return
	}
	dst, err := safePath(root, in.DstPath)
	if err != nil {
		fail(tool, in.DstPath, err)
		return
	}
	inFile, err := os.Open(src)
	if err != nil {
		fail(tool, in.SrcPath, err)
		return
	}
	defer inFile.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		fail(tool, in.DstPath, err)
		return
	}
	outFile, err := os.Create(dst)
	if err != nil {
		fail(tool, in.DstPath, err)
		return
	}
	defer outFile.Close()
	n, err := io.Copy(outFile, inFile)
	if err != nil {
		fail(tool, in.DstPath, err)
		return
	}
	writeOK(tool, in.DstPath, map[string]any{"bytes": n})
}

func runExists(root, tool, p string) {
	path, err := safePath(root, p)
	if err != nil {
		fail(tool, p, err)
		return
	}
	_, err = os.Stat(path)
	if err == nil {
		writeOK(tool, p, map[string]any{"exists": true})
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		writeOK(tool, p, map[string]any{"exists": false})
		return
	}
	fail(tool, p, err)
}

func defaultPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return "."
	}
	return p
}

func mustDecode(raw string, v any) {
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		writeErr("", "", fmt.Errorf("invalid json input: %w", err))
		os.Exit(2)
	}
}

func writeOK(tool, p string, data any) { writeJSON(Result{OK: true, Tool: tool, Path: p, Data: data}) }
func fail(tool, p string, err error)   { writeErr(tool, p, err); os.Exit(1) }
func writeErr(tool, p string, err error) {
	writeJSON(Result{OK: false, Tool: tool, Path: p, Error: err.Error()})
}
func writeJSON(v any) { enc := json.NewEncoder(os.Stdout); enc.SetIndent("", "  "); _ = enc.Encode(v) }
