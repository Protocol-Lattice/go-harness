package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string
	Path        string
	Description string
	Body        string
}

func LoadSkills(ctx context.Context, dir string) ([]Skill, error) {
	if dir == "" {
		return nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat skills dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills path is not a directory: %s", dir)
	}

	var out []Skill

	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read skill %s: %w", path, err)
		}

		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		desc := firstNonHeadingLine(b)

		out = append(out, Skill{
			Name:        name,
			Path:        path,
			Description: desc,
			Body:        strings.TrimSpace(string(b)),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func firstNonHeadingLine(b []byte) string {
	lines := bytes.Split(b, []byte("\n"))
	for _, line := range lines {
		s := strings.TrimSpace(string(line))
		if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "---") {
			continue
		}
		if len(s) > 160 {
			return s[:160] + "..."
		}
		return s
	}
	return ""
}
