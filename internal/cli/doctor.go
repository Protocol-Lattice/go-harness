package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Protocol-Lattice/go-harness/internal/harness"
)

func (a *App) doctor(ctx context.Context, args []string) error {
	cfg, _, err := a.baseFlags("doctor", args)
	if err != nil {
		return err
	}

	ok := true

	check := func(name string, err error) {
		if err != nil {
			ok = false
			fmt.Fprintf(a.stdout, "✗ %s: %v\n", name, err)
			return
		}
		fmt.Fprintf(a.stdout, "✓ %s\n", name)
	}

	_, err = os.Stat(cfg.Workspace)
	check("workspace "+cfg.Workspace, err)

	_, err = os.Stat(cfg.SkillsDir)
	if os.IsNotExist(err) {
		fmt.Fprintf(a.stdout, "• skills dir missing: %s\n", cfg.SkillsDir)
	} else {
		check("skills dir "+cfg.SkillsDir, err)
	}

	_, err = os.Stat(cfg.ProvidersFile)
	if os.IsNotExist(err) {
		fmt.Fprintf(a.stdout, "• providers file missing: %s\n", cfg.ProvidersFile)
	} else {
		check("providers file "+cfg.ProvidersFile, err)
	}

	filesystemBin := filepath.Join(".", "bin", "filesystem")
	info, err := os.Stat(filesystemBin)
	if err != nil {
		check("./bin/filesystem", err)
	} else if info.IsDir() {
		check("./bin/filesystem", fmt.Errorf("is a directory"))
	} else if info.Mode()&0o111 == 0 {
		check("./bin/filesystem", fmt.Errorf("not executable"))
	} else {
		check("./bin/filesystem", nil)
	}

	skills, err := harness.LoadSkills(ctx, cfg.SkillsDir)
	if err == nil {
		fmt.Fprintf(a.stdout, "• loaded skills: %d\n", len(skills))
	}

	if !ok {
		return fmt.Errorf("doctor found problems")
	}

	return nil
}
