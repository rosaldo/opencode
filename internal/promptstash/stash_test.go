package promptstash

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/message"
)

func TestPushPopIsLIFO(t *testing.T) {
	s := Open("")
	s.Push("first", nil)
	s.Push("second", nil)
	if s.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", s.Len())
	}
	entry, ok := s.Pop()
	if !ok || entry.Text != "second" {
		t.Fatalf("Pop() = %+v, %v; want second", entry, ok)
	}
	entry, ok = s.Pop()
	if !ok || entry.Text != "first" {
		t.Fatalf("Pop() = %+v, %v; want first", entry, ok)
	}
	if _, ok = s.Pop(); ok {
		t.Fatal("Pop() on empty stash succeeded")
	}
}

func TestPushTrimsToMaxEntries(t *testing.T) {
	s := Open("")
	for i := range MaxEntries + 10 {
		s.Push(fmt.Sprintf("p%d", i), nil)
	}
	if s.Len() != MaxEntries {
		t.Fatalf("Len() = %d, want %d", s.Len(), MaxEntries)
	}
	entries := s.List()
	if entries[0].Text != "p10" {
		t.Fatalf("oldest entry = %q, want p10", entries[0].Text)
	}
}

func TestTakeAndRemove(t *testing.T) {
	s := Open("")
	s.Push("one", nil)
	s.Push("two", nil)
	s.Push("three", nil)

	entry, ok := s.Take(1)
	if !ok || entry.Text != "two" {
		t.Fatalf("Take(1) = %+v, %v; want two", entry, ok)
	}
	if s.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", s.Len())
	}
	s.Remove(0)
	if s.List()[0].Text != "three" {
		t.Fatalf("remaining entry = %q, want three", s.List()[0].Text)
	}
	s.Remove(5) // out of range: no-op
	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}
	if _, ok := s.Take(-1); ok {
		t.Fatal("Take(-1) succeeded")
	}
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	s.Push("x", nil)
	s.Remove(0)
	if s.Len() != 0 || s.List() != nil {
		t.Fatal("nil store should stay empty")
	}
	if _, ok := s.Pop(); ok {
		t.Fatal("nil store Pop succeeded")
	}
	if _, ok := s.Take(0); ok {
		t.Fatal("nil store Take succeeded")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	atts := []message.Attachment{
		{FilePath: "/tmp/a.txt", FileName: "a.txt", MimeType: "text/plain", Content: []byte("héllo")},
	}
	s.Push("with attachment", atts)
	s.Push("plain", nil)

	reopened := Open(dir)
	if reopened.Len() != 2 {
		t.Fatalf("reopened Len() = %d, want 2", reopened.Len())
	}
	entries := reopened.List()
	if entries[0].Text != "with attachment" || len(entries[0].Attachments) != 1 {
		t.Fatalf("entries[0] = %+v", entries[0])
	}
	got := entries[0].Attachments[0]
	if got.FileName != "a.txt" || string(got.Content) != "héllo" {
		t.Fatalf("attachment = %+v", got)
	}
	if entries[0].Timestamp == 0 {
		t.Fatal("timestamp lost")
	}

	// Pop rewrites the file; only the first entry survives on disk.
	reopened.Pop()
	again := Open(dir)
	if again.Len() != 1 || again.List()[0].Text != "with attachment" {
		t.Fatalf("after pop reopen: %+v", again.List())
	}
}

func TestEmptyDirIsMemoryOnly(t *testing.T) {
	s := Open("")
	s.Push("x", nil)
	if s.path != "" {
		t.Fatalf("path = %q, want empty", s.path)
	}
}

func TestLoadToleratesMalformedAndLegacyRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	content := "not json\n" +
		`{"prompt":{"text":"from reference"},"timestamp":1700000000000}` + "\n" +
		`{"text":"ours","timestamp":1700000001000}` + "\n" +
		`{"text":"no timestamp"}` + "\n" +
		`{"text":"bad timestamp","timestamp":"oops"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(dir)
	entries := s.List()
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2 (%+v)", len(entries), entries)
	}
	if entries[0].Text != "from reference" || entries[0].Timestamp != 1700000000000 {
		t.Fatalf("entries[0] = %+v", entries[0])
	}
	if entries[1].Text != "ours" {
		t.Fatalf("entries[1] = %+v", entries[1])
	}
}

func TestLoadTrimsToMaxEntries(t *testing.T) {
	dir := t.TempDir()
	var buf []byte
	for i := range MaxEntries + 5 {
		buf = append(buf, []byte(fmt.Sprintf(`{"text":"entry%d","timestamp":%d}`+"\n", i, 1000+i))...)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), buf, 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(dir)
	if s.Len() != MaxEntries {
		t.Fatalf("Len() = %d, want %d", s.Len(), MaxEntries)
	}
	if s.List()[0].Text != "entry5" {
		t.Fatalf("oldest = %q, want entry5", s.List()[0].Text)
	}
}
