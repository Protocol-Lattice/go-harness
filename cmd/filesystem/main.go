package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Request struct {
	Tool      string          `json:"tool"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Args      json.RawMessage `json:"args"`
}

type Response struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type ListArgs struct {
	Path string `json:"path"`
}

type ReadArgs struct {
	Path string `json:"path"`
}

type WriteArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type MkdirArgs struct {
	Path string `json:"path"`
}

type RemoveArgs struct {
	Path string `json:"path"`
}

func main() {
	var root string
	flag.StringVar(&root, "root", ".", "filesystem root")
	flag.Parse()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		writeErr(err)
		os.Exit(1)
	}

	var req Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		writeErr(fmt.Errorf("decode request: %w", err))
		os.Exit(1)
	}

	result, err := dispatch(absRoot, req)
	if err != nil {
		writeErr(err)
		os.Exit(1)
	}

	writeOK(result)
}

func dispatch(root string, req Request) (any, error) {
	tool := req.Tool
	if tool == "" {
		tool = req.Name
	}
	if tool == "" {
		return nil, errors.New("missing tool name")
	}

	args := req.Arguments
	if len(args) == 0 {
		args = req.Args
	}

	switch tool {
	case "fs.list", "filesystem.list", "list":
		var a ListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		return fsList(root, a.Path)

	case "fs.read", "filesystem.read", "read":
		var a ReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		return fsRead(root, a.Path)

	case "fs.write", "filesystem.write", "write":
		var a WriteArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		return fsWrite(root, a.Path, a.Content)

	case "fs.mkdir", "filesystem.mkdir", "mkdir":
		var a MkdirArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		return fsMkdir(root, a.Path)

	case "fs.remove", "filesystem.remove", "remove":
		var a RemoveArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		return fsRemove(root, a.Path)

	default:
		return nil, fmt.Errorf("unknown tool: %s", tool)
	}
}

func fsList(root, p string) (any, error) {
	target, err := safePath(root, p)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}

	type Entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size,omitempty"`
	}

	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}

		rel, err := filepath.Rel(root, filepath.Join(target, e.Name()))
		if err != nil {
			return nil, err
		}

		out = append(out, Entry{
			Name:  e.Name(),
			Path:  filepath.ToSlash(rel),
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}

	return out, nil
}

func fsRead(root, p string) (any, error) {
	target, err := safePath(root, p)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("cannot read directory: %s", p)
	}

	b, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"path":    filepath.ToSlash(p),
		"content": string(b),
		"size":    len(b),
	}, nil
}

func fsWrite(root, p, content string) (any, error) {
	target, err := safePath(root, p)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return nil, err
	}

	return map[string]any{
		"path":  filepath.ToSlash(p),
		"bytes": len(content),
	}, nil
}

func fsMkdir(root, p string) (any, error) {
	target, err := safePath(root, p)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, err
	}

	return map[string]any{
		"path": filepath.ToSlash(p),
	}, nil
}

func fsRemove(root, p string) (any, error) {
	target, err := safePath(root, p)
	if err != nil {
		return nil, err
	}

	if target == root {
		return nil, errors.New("refusing to remove workspace root")
	}

	if err := os.RemoveAll(target); err != nil {
		return nil, err
	}

	return map[string]any{
		"path": filepath.ToSlash(p),
	}, nil
}

func safePath(root, p string) (string, error) {
	if p == "" {
		p = "."
	}

	clean := filepath.Clean(p)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}

	target := filepath.Join(root, clean)

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(root, absTarget)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root: %s", p)
	}

	return absTarget, nil
}

func writeOK(result any) {
	_ = json.NewEncoder(os.Stdout).Encode(Response{
		OK:     true,
		Result: result,
	})
}

func writeErr(err error) {
	_ = json.NewEncoder(os.Stdout).Encode(Response{
		OK:    false,
		Error: err.Error(),
	})
}
