package git

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkChangeTotals(t *testing.T, repository *Repo, wantAdded, wantRemoved int) {
	t.Helper()
	status, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(repository.Dir, "index")
	before, err := os.ReadFile(index)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	added, removed, err := repository.ChangeTotals(status)
	if err != nil || added != wantAdded || removed != wantRemoved {
		t.Fatalf("ChangeTotals = +%d -%d, %v; want +%d -%d", added, removed, err, wantAdded, wantRemoved)
	}
	after, err := os.ReadFile(index)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("counting changed the real index")
	}
}

func TestChangeTotalsWorktreeAgainstHEAD(t *testing.T) {
	repository, command := repo(t)
	checkChangeTotals(t, repository, 0, 0)
	write(t, repository.Root, "a.txt", "intermediate\nstaged\n")
	command("add", "a.txt")
	write(t, repository.Root, "a.txt", "one\ntwo\nthree\n")
	checkChangeTotals(t, repository, 0, 0)
	write(t, repository.Root, "a.txt", "one\ntwo\nchanged\nextra\n")
	command("mv", "b.txt", "renamed.txt")
	write(t, repository.Root, "untracked/new file.txt", "first\nlast")
	write(t, repository.Root, "empty", "")
	write(t, repository.Root, "binary", "\x00\nnot lines\n")
	write(t, repository.Root, "marked.bin", "text marked binary\n")
	write(t, repository.Root, ".gitattributes", "*.bin -diff\n")
	write(t, repository.Root, ".gitignore", "ignored/\n")
	write(t, repository.Root, "ignored/data", "not counted\n")
	checkChangeTotals(t, repository, 6, 1)
}

func TestChangeTotalsRemovedFromIndex(t *testing.T) {
	repository, command := repo(t)
	command("rm", "--cached", "a.txt")
	checkChangeTotals(t, repository, 0, 0)
	write(t, repository.Root, "a.txt", "one\nthree\n")
	command("rm", "b.txt")
	checkChangeTotals(t, repository, 0, 2)
}

func TestChangeTotalsFileDirectorySwap(t *testing.T) {
	repository, command := repo(t)
	write(t, repository.Root, "dir/old", "old\nold again\n")
	command("add", "dir/old")
	command("commit", "-qm", "nested base")
	command("rm", "a.txt", "dir/old")
	write(t, repository.Root, "a.txt/new", "new\n")
	write(t, repository.Root, "dir", "replacement\n")
	checkChangeTotals(t, repository, 2, 5)
}

func TestChangeTotalsUnborn(t *testing.T) {
	repository, command := repo(t)
	command("switch", "--orphan", "unborn")
	checkChangeTotals(t, repository, 0, 0)
	write(t, repository.Root, "staged", "not on disk any more\n")
	command("add", "staged")
	write(t, repository.Root, "staged", "one\ntwo\nthree\n")
	write(t, repository.Root, "new", "one")
	checkChangeTotals(t, repository, 4, 0)
}

func TestChangeTotalsRejectsLargeText(t *testing.T) {
	repository, command := repo(t)
	name := "oversized.txt"
	path := repository.Abs(name)
	write(t, repository.Root, name, strings.Repeat("x\n", changeFileBytes/2+1))
	checkError := func() {
		t.Helper()
		status, err := repository.Status()
		if err != nil {
			t.Fatal(err)
		}
		if added, removed, err := repository.ChangeTotals(status); err == nil || added != 0 || removed != 0 {
			t.Fatalf("oversized text returned +%d -%d, %v", added, removed, err)
		}
	}
	checkError()
	command("add", name)
	checkError()
	command("commit", "-qm", "large base")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	checkError()
}
