package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Protocol-Lattice/go-harness/internal/harness"
)

func (a *App) chat(ctx context.Context, args []string) error {
	cfg, _, err := a.baseFlags("chat", args)
	if err != nil {
		return err
	}

	return harness.Run(ctx, cfg)
}

func (a *App) runTask(ctx context.Context, args []string) error {
	cfg, fs, err := a.baseFlags("run", args)
	if err != nil {
		return err
	}

	task := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if task == "" {
		return errors.New("missing task prompt")
	}

	rt, err := harness.NewRuntime(ctx, cfg, a.stdin, a.stdout)
	if err != nil {
		return err
	}

	return rt.RunOnce(ctx, task, a.stdout)
}

func (a *App) skills(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: go-harness skills <list|fetch>")
	}

	switch args[0] {
	case "list":
		cfg, _, err := a.baseFlags("skills list", args[1:])
		if err != nil {
			return err
		}

		skills, err := harness.LoadSkills(ctx, cfg.SkillsDir)
		if err != nil {
			return err
		}

		if len(skills) == 0 {
			fmt.Fprintln(a.stdout, "no skills loaded")
			return nil
		}

		for _, s := range skills {
			if s.Description == "" {
				fmt.Fprintf(a.stdout, "- %s\t%s\n", s.Name, s.Path)
				continue
			}
			fmt.Fprintf(a.stdout, "- %s\t%s\n  %s\n", s.Name, s.Path, s.Description)
		}

		return nil

	case "fetch":
		cfg, fs, err := a.baseFlags("skills fetch", args[1:])
		if err != nil {
			return err
		}

		source := strings.TrimSpace(strings.Join(fs.Args(), " "))
		if source == "" {
			return errors.New("usage: go-harness skills fetch <github-url-or-owner/repo>")
		}

		result, err := harness.FetchSkillsFromGitHub(ctx, harness.FetchSkillsOptions{
			Source: source,
			Dest:   cfg.SkillsDir,
			Token:  cfg.GitHubToken,
		})
		if err != nil {
			return err
		}

		fmt.Fprintf(
			a.stdout,
			"fetched %d skill file(s) from %s/%s@%s into %s\n",
			result.FilesWritten,
			result.Source.Owner,
			result.Source.Repo,
			result.Source.Ref,
			result.Destination,
		)

		if result.Source.Subdir != "" {
			fmt.Fprintf(a.stdout, "source subdir: %s\n", result.Source.Subdir)
		}

		return nil

	default:
		return fmt.Errorf("unknown skills command: %s", args[0])
	}
}

func (a *App) tools(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: go-harness tools list")
	}

	switch args[0] {
	case "list":
		cfg, _, err := a.baseFlags("tools list", args[1:])
		if err != nil {
			return err
		}

		rt, err := harness.NewRuntime(ctx, cfg, a.stdin, a.stdout)
		if err != nil {
			return err
		}

		return rt.ListTools(a.stdout)

	default:
		return fmt.Errorf("unknown tools command: %s", args[0])
	}
}
