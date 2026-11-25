package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"fiveleaf/internal/vfs"
)

const (
	changePathsMax   = 4096
	changeFileBytes  = 8 << 20
	changeTotalBytes = 64 << 20
)

// ChangeTotals counts the worktree against the HEAD in status, independent of
// staging. A private index holds just the affected HEAD paths and intent-to-add
// entries for new files; Git handles renames, attributes, symlinks and EOFs.
func (repository *Repo) ChangeTotals(status *Status) (added, removed int, err error) {
	if vfs.IsRemote(repository.Root) {
		return 0, 0, errors.New("line counts require a local worktree")
	}
	if status == nil {
		return 0, 0, errors.New("git status is unavailable")
	}
	if len(status.Entries) > changePathsMax {
		return 0, 0, errors.New("too many changed paths to count lines")
	}
	paths := map[string]bool{}
	for _, entry := range status.Entries {
		if !entry.Submodule() {
			paths[repository.Rel(entry.Path)] = true
			if entry.Orig != "" {
				paths[repository.Rel(entry.Orig)] = true
			}
		}
	}
	if len(paths) == 0 {
		return 0, 0, nil
	}

	var total int64
	charge := func(name string, size int64) error {
		total += size
		if size > changeFileBytes || total > changeTotalBytes {
			return fmt.Errorf("change in %q exceeds the line-count limit", name)
		}
		return nil
	}
	var names []string
	for name := range paths {
		names = append(names, name)
	}
	slices.Sort(names)
	var index bytes.Buffer
	if status.Head != "" {
		// Keep each argv below the Windows command-line limit as well.
		for chunk := names; len(chunk) != 0; {
			count, length := 0, 0
			for count < len(chunk) && length < 8192 {
				length += len(chunk[count]) + 3
				count++
			}
			rows, err := repository.run(nil, append([]string{"--literal-pathspecs", "ls-tree", "-rlz", status.Head, "--"}, chunk[:count]...)...)
			if err != nil {
				return 0, 0, err
			}
			for row := range strings.SplitSeq(strings.TrimSuffix(string(rows), "\x00"), "\x00") {
				if row == "" {
					continue
				}
				header, name, ok := strings.Cut(row, "\t")
				fields := strings.Fields(header)
				if !ok || len(fields) != 4 {
					return 0, 0, errors.New("invalid git ls-tree record")
				}
				if fields[1] != "blob" || !paths[name] {
					continue
				}
				size, err := strconv.ParseInt(fields[3], 10, 64)
				if err != nil {
					return 0, 0, err
				}
				if err := charge(name, size); err != nil {
					return 0, 0, err
				}
				fmt.Fprintf(&index, "%s %s\t%s\x00", fields[0], fields[2], name)
				paths[name] = false
			}
			chunk = chunk[count:]
		}
	}
	var newFiles bytes.Buffer
	for _, name := range names {
		info, err := os.Lstat(repository.Abs(name))
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return 0, 0, err
		}
		if info.IsDir() {
			continue
		}
		if !info.Mode().IsRegular() && info.Mode()&fs.ModeSymlink == 0 {
			return 0, 0, fmt.Errorf("cannot count lines in special file %q", name)
		}
		if err := charge(name, info.Size()); err != nil {
			return 0, 0, err
		}
		if paths[name] {
			newFiles.WriteString(name)
			newFiles.WriteByte(0)
		}
	}
	if index.Len() == 0 && newFiles.Len() == 0 {
		return 0, 0, nil
	}
	scratch, err := os.MkdirTemp("", "fiveleaf-changes-*")
	if err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(scratch)
	env := []string{"LC_ALL=C", "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1",
		"GIT_INDEX_FILE=" + filepath.Join(scratch, "index"), "GIT_OBJECT_DIRECTORY=" + scratch,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=" + strconv.Quote(filepath.Join(repository.Common, "objects"))}
	run := func(input io.Reader, args ...string) ([]byte, error) {
		args = append([]string{"-C", repository.Root, "--literal-pathspecs", "-c", "core.hooksPath=" + filepath.Join(scratch, "hooks"),
			"-c", "core.splitIndex=false", "-c", "core.sparseCheckout=false", "-c", "core.fsmonitor=false",
			"-c", "core.bigFileThreshold=" + strconv.Itoa(changeFileBytes)}, args...)
		cmd, err := vfs.Command(scratch, env, input, "git", args...)
		if err != nil {
			return nil, err
		}
		out, err := cmd.Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			err = &Error{Args: cmd.Args, Stderr: strings.TrimSpace(string(exit.Stderr)), Err: err}
		}
		return out, err
	}
	if index.Len() > 0 {
		if _, err := run(&index, "update-index", "-z", "--index-info"); err != nil {
			return 0, 0, err
		}
	}
	diff := []string{"diff-files", "--numstat", "--no-ext-diff", "--no-textconv", "--ignore-submodules=all", "-M", "-l200"}
	if newFiles.Len() > 0 {
		// Adding dir/file to the index removes a former file named dir.
		base, err := run(nil, "write-tree")
		if err != nil {
			return 0, 0, err
		}
		if _, err := run(&newFiles, "add", "-Nf", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return 0, 0, err
		}
		diff[0] = "diff-index"
		diff = append(diff, strings.TrimSpace(string(base)))
	}
	out, err := run(nil, append(diff, "--")...)
	if err != nil {
		return 0, 0, err
	}
	for row := range strings.Lines(string(out)) {
		insertions, rest, _ := strings.Cut(row, "\t")
		deletions, _, ok := strings.Cut(rest, "\t")
		if ok && insertions == "-" && deletions == "-" {
			continue
		}
		ins, insErr := strconv.Atoi(insertions)
		del, delErr := strconv.Atoi(deletions)
		if !ok || insErr != nil || delErr != nil || ins < 0 || del < 0 {
			return 0, 0, errors.New("invalid git numstat record")
		}
		added += ins
		removed += del
	}
	return added, removed, nil
}
