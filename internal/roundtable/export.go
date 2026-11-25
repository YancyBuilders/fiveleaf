package roundtable

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type ExportHistory struct {
	recent   []Message
	olderDir string
}

func (room *Room) ExportHistory() ExportHistory {
	room.mu.Lock()
	defer room.mu.Unlock()
	history := ExportHistory{recent: slices.Clone(room.messages)}
	if room.earlier && room.archive != nil {
		history.olderDir = filepath.Join(room.archive.dir, "messages")
	}
	if room.chain != nil && room.chain.task.Task != nil {
		history.recent = append(history.recent, room.chain.task)
	}
	return history
}

func (history ExportHistory) Visit(visit func(Message) error) error {
	if history.olderDir != "" {
		entries, err := os.ReadDir(history.olderDir)
		if err != nil {
			return err
		}
		end := len(entries)
		if len(history.recent) > 0 {
			cutoff := messageFile(history.recent[0])
			index, found := slices.BinarySearchFunc(entries, cutoff, func(entry os.DirEntry, name string) int { return strings.Compare(entry.Name(), name) })
			if !found {
				return errors.New("roundtable history changed during export")
			}
			end = index
		}
		for _, entry := range entries[:end] {
			if !strings.HasSuffix(entry.Name(), ".json") || !entry.Type().IsRegular() {
				continue
			}
			message, err := readArchivedMessage(history.olderDir, entry.Name())
			if err != nil {
				return err
			}
			if err := visit(message); err != nil {
				return err
			}
		}
	}
	for _, message := range history.recent {
		if err := visit(message); err != nil {
			return err
		}
	}
	return nil
}
