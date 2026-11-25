package main

import (
	"slices"
	"time"

	"fiveleaf/internal/roundtable"
	"github.com/egoist/mygo/ui"
)

type meetingTopicIndex struct {
	numbers          map[string]int
	lastID, lastRole string
}

func (index *meetingTopicIndex) record(message roundtable.Message) {
	if !message.InOverview() {
		return
	}
	if message.Role == "user" && (index.lastRole == "" || index.lastRole == "user") {
		if index.numbers == nil {
			index.numbers = make(map[string]int)
		}
		index.numbers[message.ID] = len(index.numbers) + 1
	}
	index.lastID, index.lastRole = message.ID, message.Role
}

type meetingTopicCache struct {
	meetingTopicIndex
	first, last, loading string
	scanError            string
	retryAfter           time.Time
}

func (w *roundtableWindow) flowTopics(flow ui.Element, snapshot roundtable.Snapshot, latestPage bool) map[string]int {
	cache := ui.Local(flow, "topic-index", func() meetingTopicCache { return meetingTopicCache{} })
	page := snapshot.Messages
	first, last := page[0].ID, page[len(page)-1].ID
	if cache.first == first && cache.last == last && last != "" && (cache.retryAfter.IsZero() || time.Now().Before(cache.retryAfter)) {
		return cache.numbers
	}
	cache.first, cache.last = first, last
	cache.retryAfter = time.Time{}
	room := w.meeting.room
	if room != nil && (snapshot.HasEarlier || !latestPage || len(page) >= meetingPageSize) {
		snapshot = room.Snapshot()
	}
	history := snapshot.Messages
	if len(history) == 0 {
		history = page
	}
	if cache.lastID != "" {
		at := slices.IndexFunc(history, func(message roundtable.Message) bool { return message.ID == cache.lastID })
		if at >= 0 {
			for _, message := range history[at+1:] {
				cache.record(message)
			}
			return cache.numbers
		}
	}
	if !snapshot.HasEarlier || room == nil {
		cache.meetingTopicIndex, cache.loading = meetingTopicIndex{}, ""
		for _, message := range history {
			cache.record(message)
		}
		return cache.numbers
	}
	if cache.loading == "" {
		through := history[len(history)-1].ID
		cache.loading = through
		generation := w.viewGeneration
		records := room.ExportHistory()
		go func() {
			var index meetingTopicIndex
			err := records.Visit(func(message roundtable.Message) error {
				if index.lastID != through {
					index.record(message)
				}
				return nil
			})
			w.postWindow(func() {
				if w.viewGeneration != generation || w.meeting == nil || w.meeting.room != room || cache.loading != through {
					return
				}
				cache.loading = ""
				if err != nil {
					cache.scanError = err.Error()
					w.err = cache.scanError
					cache.retryAfter = time.Now().Add(time.Second)
					return
				}
				if cache.scanError != "" && w.err == cache.scanError {
					w.err = ""
				}
				cache.scanError = ""
				cache.meetingTopicIndex, cache.first, cache.last = index, "", ""
			})
		}()
	}
	return cache.numbers
}
