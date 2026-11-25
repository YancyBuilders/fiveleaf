package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenWorktreeDirectories(t *testing.T) {
	main, git := repo(t)
	if main.Dir != main.Common {
		t.Fatalf("main worktree has different Git directories: %+v", main)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	git("worktree", "add", "-b", "linked", linked)
	for _, tc := range []struct {
		name, root string
		linked     bool
	}{
		{"main", main.Root, false},
		{"linked", linked, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alias := filepath.Join(t.TempDir(), "alias")
			if err := os.Symlink(tc.root, alias); err != nil {
				t.Skip("no symlinks:", err)
			}
			for _, root := range []string{tc.root, alias} {
				r, err := Open(root)
				if err != nil {
					t.Fatal(err)
				}
				if r.Root != root {
					t.Fatalf("root = %q, want original spelling %q", r.Root, root)
				}
				if (r.Dir != r.Common) != tc.linked {
					t.Fatalf("linked = %v, repository %+v", tc.linked, r)
				}
				if r.Common != main.Common {
					t.Fatalf("common directory = %q, want %q", r.Common, main.Common)
				}
			}
		})
	}
}
