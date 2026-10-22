package session

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"

	"github.com/opencode-ai/opencode/internal/db"
	"github.com/opencode-ai/opencode/internal/pubsub"
)

// newForkTestService opens a real SQLite database with the embedded migrations
// so the fork path exercises the trigger, the FK cascade and the generated
// queries end-to-end (Spec 09p).
func newForkTestService(t *testing.T) (Service, *db.Queries) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	goose.SetBaseFS(db.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	q := db.New(sqlDB)
	return NewService(q), q
}

// seedMessage inserts a raw message row with explicit timestamps so the tests
// can pin the copy order without sleeping between inserts.
func seedMessage(t *testing.T, q *db.Queries, id, sessionID, role string, createdAt int64, finished bool) db.Message {
	t.Helper()
	params := db.CopyMessageParams{
		ID:        id,
		SessionID: sessionID,
		Role:      role,
		Parts:     `[{"type":"text","data":{"text":"hi"}}]`,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	if finished {
		params.FinishedAt = sql.NullInt64{Int64: createdAt, Valid: true}
	}
	m, err := q.CopyMessage(context.Background(), params)
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}
	return m
}

func TestForkFullSessionCopiesSettledMessages(t *testing.T) {
	svc, q := newForkTestService(t)
	ctx := context.Background()

	parent, err := svc.Create(ctx, "Parent")
	if err != nil {
		t.Fatal(err)
	}
	seedMessage(t, q, "u1", parent.ID, "user", 100, false)
	seedMessage(t, q, "a1", parent.ID, "assistant", 200, true)
	seedMessage(t, q, "u2", parent.ID, "user", 300, false)
	// In flight: an assistant row without finished_at must not be copied.
	seedMessage(t, q, "a2", parent.ID, "assistant", 400, false)

	events := svc.Subscribe(ctx)

	fork, err := svc.Fork(ctx, parent.ID, "")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}

	if fork.ID == "" || fork.ID == parent.ID {
		t.Fatalf("fork id = %q, want a fresh id", fork.ID)
	}
	if fork.Title != "Parent (fork #1)" {
		t.Errorf("title = %q, want %q", fork.Title, "Parent (fork #1)")
	}
	if fork.ParentSessionID != "" {
		t.Errorf("ParentSessionID = %q, want empty so the fork stays listed", fork.ParentSessionID)
	}

	dbSession, err := q.GetSessionByID(ctx, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbSession.ForkSessionID.String != parent.ID {
		t.Errorf("fork_session_id = %q, want %q", dbSession.ForkSessionID.String, parent.ID)
	}
	// The insert trigger keeps the counters right (Spec 09p).
	if dbSession.MessageCount != 3 {
		t.Errorf("message_count = %d, want 3 (in-flight excluded)", dbSession.MessageCount)
	}

	copied, err := q.ListMessagesBySession(ctx, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 3 {
		t.Fatalf("copied = %d, want 3", len(copied))
	}
	want := []struct {
		id   string
		role string
		at   int64
	}{
		{"u1", "user", 100},
		{"a1", "assistant", 200},
		{"u2", "user", 300},
	}
	for i, m := range copied {
		if m.ID == want[i].id {
			t.Errorf("copied[%d].ID = %q, want a fresh id (remapped)", i, m.ID)
		}
		if m.Role != want[i].role {
			t.Errorf("copied[%d].Role = %q, want %q", i, m.Role, want[i].role)
		}
		if m.CreatedAt != want[i].at {
			t.Errorf("copied[%d].CreatedAt = %d, want %d (order preserved)", i, m.CreatedAt, want[i].at)
		}
		if m.SessionID != fork.ID {
			t.Errorf("copied[%d].SessionID = %q, want %q", i, m.SessionID, fork.ID)
		}
	}

	// Both parent and fork show up in the switcher list.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("List = %d sessions, want 2", len(list))
	}

	select {
	case ev := <-events:
		if ev.Type != pubsub.CreatedEvent || ev.Payload.ID != fork.ID {
			t.Errorf("event = %#v, want CreatedEvent for the fork", ev)
		}
	default:
		t.Error("Fork should publish a CreatedEvent")
	}
}

func TestForkBeforeMessageStopsAtBoundary(t *testing.T) {
	svc, q := newForkTestService(t)
	ctx := context.Background()

	parent, err := svc.Create(ctx, "Parent")
	if err != nil {
		t.Fatal(err)
	}
	seedMessage(t, q, "u1", parent.ID, "user", 100, false)
	seedMessage(t, q, "a1", parent.ID, "assistant", 200, true)
	seedMessage(t, q, "u2", parent.ID, "user", 300, false)

	fork, err := svc.Fork(ctx, parent.ID, "u2")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	copied, err := q.ListMessagesBySession(ctx, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 2 {
		t.Fatalf("copied = %d, want 2 (boundary excluded)", len(copied))
	}

	// Forking at the very first message has nothing before it.
	if _, err := svc.Fork(ctx, parent.ID, "u1"); !errors.Is(err, ErrSessionEmpty) {
		t.Errorf("boundary at first message: err = %v, want ErrSessionEmpty", err)
	}

	// An unknown boundary message is reported, not silently treated as full.
	if _, err := svc.Fork(ctx, parent.ID, "nope"); err == nil {
		t.Error("unknown boundary must fail")
	}
}

func TestForkEmptySession(t *testing.T) {
	svc, _ := newForkTestService(t)
	ctx := context.Background()

	empty, err := svc.Create(ctx, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fork(ctx, empty.ID, ""); !errors.Is(err, ErrSessionEmpty) {
		t.Errorf("err = %v, want ErrSessionEmpty", err)
	}
	// No fork row may be left behind.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("List = %d, want only the empty session", len(list))
	}
}

func TestForkTitleNumbering(t *testing.T) {
	svc, q := newForkTestService(t)
	ctx := context.Background()

	parent, err := svc.Create(ctx, "Parent")
	if err != nil {
		t.Fatal(err)
	}
	seedMessage(t, q, "u1", parent.ID, "user", 100, false)

	first, err := svc.Fork(ctx, parent.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Fork(ctx, parent.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Title != "Parent (fork #1)" {
		t.Errorf("first title = %q", first.Title)
	}
	if second.Title != "Parent (fork #2)" {
		t.Errorf("second title = %q, want the incremented number", second.Title)
	}
}

func TestForkMissingSession(t *testing.T) {
	svc, _ := newForkTestService(t)
	if _, err := svc.Fork(context.Background(), "missing", ""); err == nil {
		t.Error("unknown session must fail")
	}
}
