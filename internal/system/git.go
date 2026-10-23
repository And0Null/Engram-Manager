package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitRepoHealth describes the working tree and branch health of a Git repository.
type GitRepoHealth struct {
	Path        string `json:"path"`
	IsRepo      bool   `json:"is_repo"`
	Branch      string `json:"branch,omitempty"`
	IsClean     bool   `json:"is_clean"`
	DirtyCount  int    `json:"dirty_count"`
	LastCommit  string `json:"last_commit,omitempty"`
	CommitMsg   string `json:"commit_msg,omitempty"`
	CommitTime  string `json:"commit_time,omitempty"`
}

// InspectGitRepo inspects a directory and returns its Git status within a 500ms timeout.
func InspectGitRepo(dir string) GitRepoHealth {
	health := GitRepoHealth{
		Path:    dir,
		IsRepo:  false,
		IsClean: true,
	}

	if dir == "" {
		return health
	}

	absDir, err := filepath.Abs(expandPath(dir))
	if err != nil {
		return health
	}
	health.Path = absDir

	fi, err := os.Stat(absDir)
	if err != nil || !fi.IsDir() {
		return health
	}

	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	// Check if repo
	cmdCheck := exec.CommandContext(ctx, "git", "-C", absDir, "rev-parse", "--is-inside-work-tree")
	outCheck, err := cmdCheck.Output()
	if err != nil || strings.TrimSpace(string(outCheck)) != "true" {
		return health
	}
	health.IsRepo = true

	// Current branch
	cmdBranch := exec.CommandContext(ctx, "git", "-C", absDir, "rev-parse", "--abbrev-ref", "HEAD")
	if outBranch, err := cmdBranch.Output(); err == nil {
		health.Branch = strings.TrimSpace(string(outBranch))
	}

	// Status dirty check
	cmdStatus := exec.CommandContext(ctx, "git", "-C", absDir, "status", "--porcelain")
	if outStatus, err := cmdStatus.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(outStatus)), "\n")
		dirtyLines := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				dirtyLines++
			}
		}
		health.DirtyCount = dirtyLines
		health.IsClean = (dirtyLines == 0)
	}

	// Last commit info: %h|%cr|%s
	cmdLog := exec.CommandContext(ctx, "git", "-C", absDir, "log", "-1", "--format=%h|%cr|%s")
	if outLog, err := cmdLog.Output(); err == nil {
		parts := strings.Split(strings.TrimSpace(string(outLog)), "|")
		if len(parts) >= 3 {
			health.LastCommit = parts[0]
			health.CommitTime = parts[1]
			health.CommitMsg = parts[2]
		}
	}

	return health
}

func expandPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
