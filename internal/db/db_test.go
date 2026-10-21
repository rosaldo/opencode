package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"

	"github.com/pressly/goose/v3"
)

// openTestDB opens a real SQLite database in a temp dir and applies the embedded
// migrations, mirroring the production path in Connect(). It exercises the schema
// and the generated queries end-to-end, which unit fakes cannot.
func openTestDB(t *testing.T) *sql.DB {
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
	goose.SetBaseFS(FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return sqlDB
}

func TestMigrationsApplyCleanly(t *testing.T) {
	db := openTestDB(t)

	var version int64
	if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version); err != nil {
		t.Fatalf("read goose version: %v", err)
	}
	if version < 2 {
		t.Fatalf("expected migrations up to at least 2, got %d", version)
	}
}

func TestSessionCRUD(t *testing.T) {
	sqlDB := openTestDB(t)
	q := New(sqlDB)
	ctx := context.Background()

	created, err := q.CreateSession(ctx, CreateSessionParams{ID: "s1", Title: "hello"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if created.Title != "hello" || created.ID != "s1" {
		t.Fatalf("created = %#v", created)
	}

	got, err := q.GetSessionByID(ctx, "s1")
	if err != nil {
		t.Fatalf("GetSessionByID: %v", err)
	}
	if got.Title != "hello" {
		t.Errorf("title = %q", got.Title)
	}

	updated, err := q.UpdateSession(ctx, UpdateSessionParams{
		ID:               "s1",
		Title:            "renamed",
		PromptTokens:     11,
		CompletionTokens: 22,
		SummaryMessageID: sql.NullString{String: "m1", Valid: true},
		Cost:             0.5,
	})
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	if updated.Title != "renamed" || updated.PromptTokens != 11 || updated.Cost != 0.5 {
		t.Fatalf("updated = %#v", updated)
	}
	if !updated.SummaryMessageID.Valid || updated.SummaryMessageID.String != "m1" {
		t.Fatalf("summary id = %#v", updated.SummaryMessageID)
	}

	if _, err := q.CreateSession(ctx, CreateSessionParams{ID: "s2", Title: "two"}); err != nil {
		t.Fatal(err)
	}
	list, err := q.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d sessions, want 2", len(list))
	}

	if err := q.DeleteSession(ctx, "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := q.GetSessionByID(ctx, "s1"); err != sql.ErrNoRows {
		t.Fatalf("deleted session err = %v, want ErrNoRows", err)
	}
}

func TestMessageCRUDAndCascade(t *testing.T) {
	sqlDB := openTestDB(t)
	q := New(sqlDB)
	ctx := context.Background()

	if _, err := q.CreateSession(ctx, CreateSessionParams{ID: "s1", Title: "t"}); err != nil {
		t.Fatal(err)
	}
	m, err := q.CreateMessage(ctx, CreateMessageParams{
		ID:        "m1",
		SessionID: "s1",
		Role:      "assistant",
		Parts:     `[]`,
		Model:     sql.NullString{String: "gpt-4.1", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if m.ID != "m1" || m.SessionID != "s1" {
		t.Fatalf("created = %#v", m)
	}

	got, err := q.GetMessage(ctx, "m1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.Role != "assistant" || got.Model.String != "gpt-4.1" {
		t.Fatalf("got = %#v", got)
	}

	if _, err := q.CreateMessage(ctx, CreateMessageParams{ID: "m2", SessionID: "s1", Role: "user", Parts: `[]`}); err != nil {
		t.Fatal(err)
	}
	msgs, err := q.ListMessagesBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("ListMessagesBySession: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}

	if err := q.UpdateMessage(ctx, UpdateMessageParams{
		ID:         "m1",
		Parts:      `[{"type":"text","data":{"text":"hi"}}]`,
		FinishedAt: sql.NullInt64{Int64: 99, Valid: true},
	}); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}
	after, _ := q.GetMessage(ctx, "m1")
	if after.Parts != `[{"type":"text","data":{"text":"hi"}}]` {
		t.Errorf("parts = %q", after.Parts)
	}
	if !after.FinishedAt.Valid || after.FinishedAt.Int64 != 99 {
		t.Errorf("finished_at = %#v", after.FinishedAt)
	}

	// Deleting the session must cascade to its messages (foreign_keys = ON).
	if err := q.DeleteSession(ctx, "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := q.GetMessage(ctx, "m1"); err != sql.ErrNoRows {
		t.Fatalf("message should be gone after session delete, err = %v", err)
	}
}
