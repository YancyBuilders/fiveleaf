package main

import (
	"path/filepath"
	"testing"

	"fiveleaf/internal/git"
	"fiveleaf/internal/remote"
)

func TestWorktreeLabel(t *testing.T) {
	mainGit := filepath.FromSlash("/projects/editor/.git")
	for _, tc := range []struct {
		name string
		repo *git.Repo
		want string
	}{
		{"no repository", nil, ""},
		{"main checkout", &git.Repo{Root: filepath.FromSlash("/projects/editor"), Dir: mainGit, Common: mainGit}, "main"},
		{"linked checkout", &git.Repo{Root: filepath.FromSlash("/projects/editor.worktrees/feature-login"), Dir: filepath.Join(mainGit, "worktrees", "feature-login"), Common: mainGit}, "feature-login"},
		{"remote main", &git.Repo{Root: remote.Path("offline.invalid", "/srv/editor"), Dir: remote.Path("offline.invalid", "/srv/editor/.git"), Common: remote.Path("offline.invalid", "/srv/editor/.git")}, "main"},
		{"remote linked", &git.Repo{Root: remote.Path("offline.invalid", "/srv/editor.worktrees/feature-login"), Dir: remote.Path("offline.invalid", "/srv/editor/.git/worktrees/feature-login"), Common: remote.Path("offline.invalid", "/srv/editor/.git")}, "feature-login"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &gitContext{git: &gitState{repo: tc.repo, status: &git.Status{Branch: "master"}}}
			if got := ctx.worktreeLabel(); got != tc.want {
				t.Fatalf("worktree label = %q, want %q", got, tc.want)
			}
			if tc.repo != nil && ctx.branch() != "master" {
				t.Fatal("the worktree label replaced the actual branch name")
			}
		})
	}
}
