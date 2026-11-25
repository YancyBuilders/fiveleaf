package roundtable

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
)

type dayBlock struct {
	start, end int64
	digest     string
}

var errEditedDay = errors.New("the group memory block was edited; the Markdown file was kept")

func findDayBlock(file *os.File, begin, end string) (*dayBlock, error) {
	reader := bufio.NewReader(file)
	sum := sha256.New()
	var block *dayBlock
	var offset int64
	lineStart := true
	for {
		part, err := reader.ReadSlice('\n')
		switch {
		case block == nil && lineStart && bytes.HasPrefix(part, []byte(begin)):
			digest, ok := strings.CutSuffix(string(part[len(begin):]), " -->\n")
			if !ok || len(digest) != sha256.Size*2 {
				return nil, errEditedDay
			}
			block = &dayBlock{start: offset, digest: digest}
		case block != nil:
			if lineStart && string(part) == end && hex.EncodeToString(sum.Sum(nil)) == block.digest {
				block.end = offset + int64(len(part))
				return block, nil
			}
			sum.Write(part)
		}
		offset += int64(len(part))
		lineStart = err == nil
		switch err {
		case nil, bufio.ErrBufferFull:
		case io.EOF:
			if block != nil {
				return nil, errEditedDay
			}
			return nil, nil
		default:
			return nil, err
		}
	}
}

func unchangedDay(root *os.Root, name string, original fs.FileInfo) error {
	current, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(current, original) || current.Size() != original.Size() || !current.ModTime().Equal(original.ModTime()) {
		return errEditedDay
	}
	return nil
}

func updateDayFile(root *os.Root, groupID, date string, render func(io.Writer) error) error {
	name := date + ".md"
	begin := "<!-- fiveleaf:group-memory " + groupID + " " + date + " "
	end := "<!-- /fiveleaf:group-memory " + groupID + " " + date + " -->\n"
	var original *os.File
	var block dayBlock
	info, err := root.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if render == nil {
			return nil
		}
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return errors.New("group memory requires a regular Markdown file")
	default:
		original, err = root.Open(name)
		if err != nil {
			return err
		}
		defer original.Close()
		opened, err := original.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(opened, info) {
			return errEditedDay
		}
		found, err := findDayBlock(original, begin, end)
		if err != nil {
			return err
		}
		if found == nil {
			if render == nil {
				return nil
			}
			return errors.New("a Markdown file already exists here; its contents were kept")
		}
		block = *found
		if render == nil && block.start == 0 && block.end == info.Size() {
			if err := unchangedDay(root, name, info); err != nil {
				return err
			}
			return root.Remove(name)
		}
	}
	pending := ".fiveleaf-" + rand.Text()
	file, err := root.OpenFile(pending, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(pending)
	defer file.Close()
	if block.start > 0 {
		if _, err := io.CopyN(file, io.NewSectionReader(original, 0, block.start), block.start); err != nil {
			return err
		}
	}
	if render != nil {
		if _, err := file.WriteString(begin + strings.Repeat("0", sha256.Size*2) + " -->\n"); err != nil {
			return err
		}
		sum := sha256.New()
		if err := render(io.MultiWriter(file, sum)); err != nil {
			return err
		}
		digest := hex.EncodeToString(sum.Sum(nil))
		if digest == block.digest {
			return nil
		}
		if _, err := file.WriteAt([]byte(digest), block.start+int64(len(begin))); err != nil {
			return err
		}
		if _, err := file.WriteString(end); err != nil {
			return err
		}
	}
	if original != nil {
		tail := info.Size() - block.end
		if _, err := io.CopyN(file, io.NewSectionReader(original, block.end, tail), tail); err != nil {
			return err
		}
		if err := file.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if original == nil {
		return root.Link(pending, name)
	}
	if err := unchangedDay(root, name, info); err != nil {
		return err
	}
	return root.Rename(pending, name)
}
