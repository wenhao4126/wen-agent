// Package agent — git worktree isolation for sub-agents. When a sub-agent runs
// with isolation="worktree", it operates in a temporary git worktree — an
// isolated checkout of the repository at the current HEAD. Changes made inside
// the worktree never touch the main workspace until explicitly merged.
//
// Lifecycle:
//   1. Create a temp worktree from the current branch/HEAD
//   2. Run the sub-agent inside that worktree
//   3. On completion: if the worktree is dirty, commit changes to a temp branch
//      and return the branch name for the user to review/merge. If clean,
//      remove the worktree automatically.
//
// This mirrors Claude Code's AgentTool isolation: "worktree" mode.

package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Worktree is a temporary git worktree for isolated sub-agent execution.
type Worktree struct {
	// Path is the absolute path to the worktree directory.
	Path string
	// Branch is the temp branch name created for this worktree.
	Branch string
	// BaseBranch is the branch the worktree was created from.
	BaseBranch string
	// repo is the path to the main repository.
	repo string
}

// NewWorktree creates a temporary git worktree from the current repository.
// dir is the parent directory where the worktree will be created (typically a
// temp dir). Returns nil if the repo is not a git repository or creation fails.
func NewWorktree(ctx context.Context, repoDir, tempParentDir string) (*Worktree, error) {
	if !isGitRepo(repoDir) {
		return nil, fmt.Errorf("not a git repository: %s", repoDir)
	}

	// Get the current branch name.
	baseBranch := gitBranch(repoDir)
	if baseBranch == "" {
		baseBranch = "HEAD"
	}

	// Create a unique temp branch name.
	branch := fmt.Sprintf("wenhao-worktree-%s", time.Now().Format("20060102-150405"))
	treePath := filepath.Join(tempParentDir, branch)

	// Create the worktree.
	if err := gitWorktreeAdd(ctx, repoDir, treePath, baseBranch); err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	// Create the branch in the worktree (detached from the base) so changes
	// can be committed without affecting the main repo's branches.
	if err := gitCheckoutNewBranch(ctx, treePath, branch); err != nil {
		// Best-effort cleanup on failure.
		gitWorktreeRemove(ctx, repoDir, treePath)
		return nil, fmt.Errorf("checkout new branch: %w", err)
	}

	return &Worktree{
		Path:       treePath,
		Branch:     branch,
		BaseBranch: baseBranch,
		repo:       repoDir,
	}, nil
}

// Cleanup removes the worktree. If the worktree has uncommitted changes, they
// are committed to the temp branch first. If the worktree is clean (no changes
// from the base), the branch and worktree are removed entirely.
func (w *Worktree) Cleanup(ctx context.Context) (dirty bool, _ error) {
	if w == nil || w.Path == "" {
		return false, nil
	}

	// Check if there are any changes in the worktree.
	hasChanges, err := gitHasChanges(ctx, w.Path)
	if err != nil {
		return false, fmt.Errorf("check changes: %w", err)
	}

	if hasChanges {
		// Commit changes to the temp branch so they're not lost.
		if err := gitCommitAll(ctx, w.Path, "wenhao: worktree changes"); err != nil {
			return true, fmt.Errorf("commit changes: %w", err)
		}
	}

	// Remove the worktree.
	if err := gitWorktreeRemove(ctx, w.repo, w.Path); err != nil {
		return hasChanges, fmt.Errorf("remove worktree: %w", err)
	}

	if !hasChanges {
		// Delete the temp branch since there's nothing worth keeping.
		gitBranchDelete(ctx, w.repo, w.Branch)
	}

	return hasChanges, nil
}

// Result returns a human-readable summary of the worktree outcome, suitable
// for returning to the orchestrator as part of the sub-agent result.
func (w *Worktree) Result(dirty bool) string {
	if !dirty {
		return fmt.Sprintf("(worktree %s cleaned up — no changes)", w.Branch)
	}
	return fmt.Sprintf(
		"Worktree changes saved to branch %q. Review with:\n  git -C %s log %s..%s\n  git -C %s merge %s",
		w.Branch, w.repo, w.BaseBranch, w.Branch, w.repo, w.Branch,
	)
}

// isGitRepo checks whether dir is inside a git working tree.
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// gitBranch returns the current branch name or HEAD.
func gitBranch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitWorktreeAdd creates a new worktree at path from the given ref.
func gitWorktreeAdd(ctx context.Context, repo, path, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", "--detach", path, ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// gitCheckoutNewBranch creates and checks out a new branch in the given repo.
func gitCheckoutNewBranch(ctx context.Context, repo, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "checkout", "-b", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// gitHasChanges reports whether the worktree has uncommitted changes.
func gitHasChanges(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

// gitCommitAll stages and commits all changes in the repo.
func gitCommitAll(ctx context.Context, dir, message string) error {
	// Stage everything.
	add := exec.CommandContext(ctx, "git", "-C", dir, "add", "-A")
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// Commit.
	commit := exec.CommandContext(ctx, "git", "-C", dir, "commit", "-m", message)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// gitWorktreeRemove removes a worktree and prunes the metadata.
func gitWorktreeRemove(ctx context.Context, repo, path string) error {
	// Remove the worktree directory.
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove tree: %w", err)
	}
	// Prune the worktree metadata.
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "prune")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("prune: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// gitBranchDelete deletes a branch (best-effort, errors are ignored).
func gitBranchDelete(ctx context.Context, repo, branch string) {
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "branch", "-D", branch)
	cmd.Run() // best-effort
}

// TempWorktreeDir returns the parent directory for worktree creation.
// Defaults to the OS temp dir; can be overridden via WENHAO_WORKTREE_DIR.
func TempWorktreeDir() string {
	if d := os.Getenv("WENHAO_WORKTREE_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}
