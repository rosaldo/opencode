package session

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/opencode-ai/opencode/internal/db"
	"github.com/opencode-ai/opencode/internal/pubsub"
)

// fakeQuerier implements only the session methods of db.Querier.
type fakeQuerier struct {
	db.Querier
	sessions map[string]db.Session
	order    []string
	err      error
}

func newFakeQuerier() *fakeQuerier {
	return &fakeQuerier{sessions: map[string]db.Session{}}
}

func (f *fakeQuerier) CreateSession(_ context.Context, arg db.CreateSessionParams) (db.Session, error) {
	if f.err != nil {
		return db.Session{}, f.err
	}
	s := db.Session{
		ID:               arg.ID,
		ParentSessionID:  arg.ParentSessionID,
		Title:            arg.Title,
		MessageCount:     arg.MessageCount,
		PromptTokens:     arg.PromptTokens,
		CompletionTokens: arg.CompletionTokens,
		Cost:             arg.Cost,
		CreatedAt:        1,
		UpdatedAt:        1,
	}
	f.sessions[arg.ID] = s
	f.order = append(f.order, arg.ID)
	return s, nil
}

func (f *fakeQuerier) GetSessionByID(_ context.Context, id string) (db.Session, error) {
	if f.err != nil {
		return db.Session{}, f.err
	}
	s, ok := f.sessions[id]
	if !ok {
		return db.Session{}, sql.ErrNoRows
	}
	return s, nil
}

func (f *fakeQuerier) ListSessions(_ context.Context) ([]db.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []db.Session
	for _, id := range f.order {
		out = append(out, f.sessions[id])
	}
	return out, nil
}

func (f *fakeQuerier) UpdateSession(_ context.Context, arg db.UpdateSessionParams) (db.Session, error) {
	if f.err != nil {
		return db.Session{}, f.err
	}
	s, ok := f.sessions[arg.ID]
	if !ok {
		return db.Session{}, sql.ErrNoRows
	}
	s.Title = arg.Title
	s.PromptTokens = arg.PromptTokens
	s.CompletionTokens = arg.CompletionTokens
	s.SummaryMessageID = arg.SummaryMessageID
	s.Cost = arg.Cost
	f.sessions[arg.ID] = s
	return s, nil
}

func (f *fakeQuerier) DeleteSession(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.sessions, id)
	return nil
}

func TestSessionCreate(t *testing.T) {
	svc := NewService(newFakeQuerier())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := svc.Subscribe(ctx)

	s, err := svc.Create(ctx, "My Session")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.ID == "" || s.Title != "My Session" {
		t.Fatalf("unexpected session %#v", s)
	}

	select {
	case ev := <-events:
		if ev.Type != pubsub.CreatedEvent || ev.Payload.ID != s.ID {
			t.Fatalf("event = %#v", ev)
		}
	default:
		t.Fatal("Create should publish a CreatedEvent")
	}
}

func TestSessionDerivedIDs(t *testing.T) {
	svc := NewService(newFakeQuerier())
	ctx := context.Background()

	title, err := svc.CreateTitleSession(ctx, "parent-1")
	if err != nil {
		t.Fatal(err)
	}
	if title.ID != "title-parent-1" || title.ParentSessionID != "parent-1" {
		t.Fatalf("title session = %#v", title)
	}

	task, err := svc.CreateTaskSession(ctx, "call-9", "parent-1", "do work")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "call-9" || task.ParentSessionID != "parent-1" || task.Title != "do work" {
		t.Fatalf("task session = %#v", task)
	}
}

func TestSessionGetListSaveDelete(t *testing.T) {
	svc := NewService(newFakeQuerier())
	ctx := context.Background()

	s, err := svc.Create(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "second"); err != nil {
		t.Fatal(err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d, want 2", len(list))
	}

	s.Title = "renamed"
	s.PromptTokens = 10
	s.CompletionTokens = 20
	s.SummaryMessageID = "msg-1"
	s.Cost = 1.5
	saved, err := svc.Save(ctx, s)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Title != "renamed" || saved.PromptTokens != 10 || saved.Cost != 1.5 || saved.SummaryMessageID != "msg-1" {
		t.Fatalf("saved = %#v", saved)
	}

	if err := svc.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, s.ID); err == nil {
		t.Fatal("deleted session should be gone")
	}
}

func TestSessionSaveClearsSummaryID(t *testing.T) {
	svc := NewService(newFakeQuerier())
	ctx := context.Background()

	s, err := svc.Create(ctx, "t")
	if err != nil {
		t.Fatal(err)
	}
	s.SummaryMessageID = ""
	saved, err := svc.Save(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SummaryMessageID != "" {
		t.Fatalf("empty summary id should round-trip as empty, got %q", saved.SummaryMessageID)
	}
}

func TestSessionCreateError(t *testing.T) {
	q := newFakeQuerier()
	q.err = errors.New("boom")
	svc := NewService(q)
	if _, err := svc.Create(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}
