package system

import (
	"os"
	"testing"
)

func TestInspectGitRepo_Current(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	h := InspectGitRepo(cwd)
	if !h.IsRepo {
		t.Errorf("expected current dir to be recognized as git repo")
	}
	if h.Branch == "" {
		t.Errorf("expected non-empty branch")
	}
	if h.LastCommit == "" {
		t.Errorf("expected non-empty last commit")
	}
}

func TestInspectGitRepo_NonExistent(t *testing.T) {
	h := InspectGitRepo("/tmp/non-existent-directory-xyz-1234")
	if h.IsRepo {
		t.Errorf("expected is_repo false for non-existent path")
	}
}
