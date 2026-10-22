// Package promptstash stores prompts parked from the composer (Spec 09q).
// It mirrors the TypeScript reference (prompt-stash.jsonl): a bounded,
// most-recent-last list persisted as one JSON object per line, rewritten in
// full on every mutation. A Store with no path is memory-only (tests).
package promptstash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/opencode-ai/opencode/internal/logging"
	"github.com/opencode-ai/opencode/internal/message"
)

// MaxEntries is the stash capacity; pushing beyond it drops the oldest entries.
const MaxEntries = 50

const fileName = "prompt-stash.jsonl"

// Entry is one stashed prompt: its text, the attachments it carried and when
// it was stashed (unix milliseconds, as the TypeScript reference writes).
type Entry struct {
	Text        string
	Attachments []message.Attachment
	Timestamp   int64
}

// Store is the stash. List order is oldest first; Pop takes the newest.
type Store struct {
	path    string // empty = memory only
	entries []Entry
}

// Open loads the stash from dir/prompt-stash.jsonl. An empty dir yields a
// memory-only store; a missing or unreadable file yields an empty one.
func Open(dir string) *Store {
	s := &Store{}
	if dir == "" {
		return s
	}
	s.path = filepath.Join(dir, fileName)
	s.load()
	return s
}

// List returns a copy of the entries, oldest first.
func (s *Store) List() []Entry {
	if s == nil {
		return nil
	}
	return append([]Entry(nil), s.entries...)
}

// Len reports how many entries are stashed.
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.entries)
}

// Push appends an entry, trimming the oldest ones past MaxEntries.
func (s *Store) Push(text string, attachments []message.Attachment) {
	if s == nil {
		return
	}
	entry := Entry{
		Text:        text,
		Attachments: append([]message.Attachment(nil), attachments...),
		Timestamp:   time.Now().UnixMilli(),
	}
	s.entries = append(s.entries, entry)
	if len(s.entries) > MaxEntries {
		s.entries = append([]Entry(nil), s.entries[len(s.entries)-MaxEntries:]...)
	}
	s.save()
}

// Pop removes and returns the newest entry.
func (s *Store) Pop() (Entry, bool) {
	if s == nil || len(s.entries) == 0 {
		return Entry{}, false
	}
	entry := s.entries[len(s.entries)-1]
	s.entries = s.entries[:len(s.entries)-1]
	s.save()
	return entry, true
}

// Take removes and returns the entry at index (oldest = 0).
func (s *Store) Take(index int) (Entry, bool) {
	if s == nil || index < 0 || index >= len(s.entries) {
		return Entry{}, false
	}
	entry := s.entries[index]
	s.entries = append(s.entries[:index], s.entries[index+1:]...)
	s.save()
	return entry, true
}

// Remove drops the entry at index (oldest = 0).
func (s *Store) Remove(index int) {
	if s == nil {
		return
	}
	s.Take(index)
}

// record is the on-disk line shape. Prompt mirrors the TypeScript entry
// ({"prompt":{"text":...},"timestamp":...}) so the reference's file can be
// read; our own writer uses Text directly. Attachments are only written by
// this fork.
type record struct {
	Prompt      *promptField       `json:"prompt,omitempty"`
	Text        string             `json:"text,omitempty"`
	Attachments []attachmentRecord `json:"attachments,omitempty"`
	Timestamp   int64              `json:"timestamp,omitempty"`
}

type promptField struct {
	Text string `json:"text"`
}

type attachmentRecord struct {
	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	Content  []byte `json:"content"`
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Warn("prompt stash: cannot read %s: %v", s.path, err)
		}
		return
	}
	var entries []Entry
	for _, line := range splitLines(data) {
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue // tolerate malformed lines like the reference does
		}
		text := rec.Text
		if text == "" && rec.Prompt != nil {
			text = rec.Prompt.Text
		}
		if rec.Timestamp == 0 {
			continue // entries without a timestamp are not ours
		}
		atts := make([]message.Attachment, 0, len(rec.Attachments))
		for _, a := range rec.Attachments {
			atts = append(atts, message.Attachment{
				FilePath: a.FilePath,
				FileName: a.FileName,
				MimeType: a.MimeType,
				Content:  a.Content,
			})
		}
		entries = append(entries, Entry{
			Text:        text,
			Attachments: atts,
			Timestamp:   rec.Timestamp,
		})
	}
	if len(entries) > MaxEntries {
		entries = entries[len(entries)-MaxEntries:]
	}
	s.entries = entries
}

func (s *Store) save() {
	if s.path == "" {
		return
	}
	var buf []byte
	for _, entry := range s.entries {
		rec := record{
			Text:      entry.Text,
			Timestamp: entry.Timestamp,
		}
		for _, a := range entry.Attachments {
			rec.Attachments = append(rec.Attachments, attachmentRecord{
				FilePath: a.FilePath,
				FileName: a.FileName,
				MimeType: a.MimeType,
				Content:  a.Content,
			})
		}
		line, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(s.path, buf, 0o600); err != nil {
		logging.Warn("prompt stash: cannot write %s: %v", s.path, err)
	}
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		n := -1
		for i, b := range data {
			if b == '\n' {
				n = i
				break
			}
		}
		if n < 0 {
			if len(data) > 0 {
				lines = append(lines, data)
			}
			break
		}
		if n > 0 {
			lines = append(lines, data[:n])
		}
		data = data[n+1:]
	}
	return lines
}
