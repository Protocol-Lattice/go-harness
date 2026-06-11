package harness

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type GitHubSkillSource struct {
	Owner  string
	Repo   string
	Ref    string
	Subdir string
}

type FetchSkillsOptions struct {
	Source string
	Dest   string
	Token  string
	Client *http.Client
}

type FetchSkillsResult struct {
	Source       GitHubSkillSource
	FilesWritten int
	Destination  string
}

func FetchSkillsFromGitHub(ctx context.Context, opts FetchSkillsOptions) (FetchSkillsResult, error) {
	if opts.Source == "" {
		return FetchSkillsResult{}, errors.New("missing GitHub source")
	}
	if opts.Dest == "" {
		opts.Dest = "./skills"
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 60 * time.Second}
	}

	src, err := ParseGitHubSkillSource(opts.Source)
	if err != nil {
		return FetchSkillsResult{}, err
	}

	if src.Ref == "" {
		ref, err := resolveDefaultBranch(ctx, opts.Client, src, opts.Token)
		if err != nil {
			return FetchSkillsResult{}, err
		}
		src.Ref = ref
	}

	archiveURL := fmt.Sprintf(
		"https://github.com/%s/%s/archive/refs/heads/%s.zip",
		url.PathEscape(src.Owner),
		url.PathEscape(src.Repo),
		url.PathEscape(src.Ref),
	)

	body, err := httpGet(ctx, opts.Client, archiveURL, opts.Token)
	if err != nil {
		return FetchSkillsResult{}, err
	}

	files, err := extractMarkdownSkills(body, opts.Dest, src)
	if err != nil {
		return FetchSkillsResult{}, err
	}

	return FetchSkillsResult{
		Source:       src,
		FilesWritten: files,
		Destination:  opts.Dest,
	}, nil
}

func ParseGitHubSkillSource(raw string) (GitHubSkillSource, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".git")

	if raw == "" {
		return GitHubSkillSource{}, errors.New("empty GitHub source")
	}

	if !strings.Contains(raw, "://") && strings.Count(raw, "/") >= 1 {
		parts := strings.Split(raw, "/")
		if len(parts) < 2 {
			return GitHubSkillSource{}, fmt.Errorf("invalid GitHub source: %s", raw)
		}
		return GitHubSkillSource{
			Owner: parts[0],
			Repo:  parts[1],
		}, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return GitHubSkillSource{}, fmt.Errorf("parse GitHub URL: %w", err)
	}
	if u.Host != "github.com" && u.Host != "www.github.com" {
		return GitHubSkillSource{}, fmt.Errorf("not a github.com URL: %s", raw)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return GitHubSkillSource{}, fmt.Errorf("invalid GitHub repository URL: %s", raw)
	}

	src := GitHubSkillSource{
		Owner: parts[0],
		Repo:  strings.TrimSuffix(parts[1], ".git"),
	}

	// Supported:
	// https://github.com/owner/repo
	// https://github.com/owner/repo/tree/main
	// https://github.com/owner/repo/tree/main/skills
	if len(parts) >= 4 && parts[2] == "tree" {
		src.Ref = parts[3]
		if len(parts) > 4 {
			src.Subdir = path.Join(parts[4:]...)
		}
	}

	return src, nil
}

func resolveDefaultBranch(ctx context.Context, client *http.Client, src GitHubSkillSource, token string) (string, error) {
	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s",
		url.PathEscape(src.Owner),
		url.PathEscape(src.Repo),
	)

	body, err := httpGet(ctx, client, apiURL, token)
	if err != nil {
		return "", err
	}

	var payload struct {
		DefaultBranch string `json:"default_branch"`
		Message       string `json:"message"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode GitHub repository metadata: %w", err)
	}

	if payload.DefaultBranch == "" {
		if payload.Message != "" {
			return "", fmt.Errorf("GitHub API error: %s", payload.Message)
		}
		return "", errors.New("GitHub repository metadata did not include default_branch")
	}

	return payload.DefaultBranch, nil
}

func httpGet(ctx context.Context, client *http.Client, rawURL string, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "go-harness")
	req.Header.Set("Accept", "application/vnd.github+json, application/zip")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GET %s failed: %s: %s", rawURL, resp.Status, strings.TrimSpace(string(limited)))
	}

	return io.ReadAll(resp.Body)
}

func extractMarkdownSkills(zipBytes []byte, dest string, src GitHubSkillSource) (int, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return 0, fmt.Errorf("open GitHub zip archive: %w", err)
	}

	absDest, err := filepath.Abs(dest)
	if err != nil {
		return 0, err
	}

	if err := os.MkdirAll(absDest, 0o755); err != nil {
		return 0, err
	}

	prefix := src.Repo + "-" + src.Ref + "/"
	subdir := strings.Trim(src.Subdir, "/")
	if subdir != "" {
		prefix += subdir + "/"
	}

	written := 0

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		name := filepath.ToSlash(file.Name)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !strings.EqualFold(path.Ext(name), ".md") {
			continue
		}

		rel := strings.TrimPrefix(name, prefix)
		if rel == "" || strings.HasPrefix(rel, "../") {
			continue
		}

		target := filepath.Join(absDest, filepath.FromSlash(rel))
		if !isInsideDir(absDest, target) {
			return written, fmt.Errorf("refusing unsafe archive path: %s", file.Name)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, err
		}

		rc, err := file.Open()
		if err != nil {
			return written, err
		}

		err = writeFileFromReader(target, rc)
		closeErr := rc.Close()
		if err != nil {
			return written, err
		}
		if closeErr != nil {
			return written, closeErr
		}

		written++
	}

	if written == 0 {
		return 0, fmt.Errorf("no markdown skills found in %s/%s ref=%s subdir=%q", src.Owner, src.Repo, src.Ref, src.Subdir)
	}

	return written, nil
}

func writeFileFromReader(target string, r io.Reader) error {
	tmp := target + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()

	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}

	return os.Rename(tmp, target)
}

func isInsideDir(root, target string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false
	}

	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}
