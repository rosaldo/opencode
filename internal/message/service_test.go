package message

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/opencode-ai/opencode/internal/db"
	"github.com/opencode-ai/opencode/internal/pubsub"
)

// fakeQuerier is a minimal in-memory db.Querier; only the message methods are
// implemented, the rest panic so a test that drifts into them fails loudly.
type fakeQuerier struct {
	db.Querier
	messages map[string]db.Message
	order    []string
	err      error
}

func newFakeQuerier() *fakeQuerier {
	return &fakeQuerier{messages: map[string]db.Message{}}
}

func (f *fakeQuerier) CreateMessage(_ context.Context, arg db.CreateMessageParams) (db.Message, error) {
	if f.err != nil {
		return db.Message{}, f.err
	}
	m := db.Message{
		ID:        arg.ID,
		SessionID: arg.SessionID,
		Role:      arg.Role,
		Parts:     arg.Parts,
		Model:     arg.Model,
		CreatedAt: 1,
		UpdatedAt: 1,
	}
	f.messages[arg.ID] = m
	f.order = append(f.order, arg.ID)
	return m, nil
}

func (f *fakeQuerier) GetMessage(_ context.Context, id string) (db.Message, error) {
	if f.err != nil {
		return db.Message{}, f.err
	}
	m, ok := f.messages[id]
	if !ok {
		return db.Message{}, sql.ErrNoRows
	}
	return m, nil
}

func (f *fakeQuerier) ListMessagesBySession(_ context.Context, sessionID string) ([]db.Message, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []db.Message
	for _, id := range f.order {
		if m := f.messages[id]; m.SessionID == sessionID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeQuerier) UpdateMessage(_ context.Context, arg db.UpdateMessageParams) error {
	if f.err != nil {
		return f.err
	}
	m, ok := f.messages[arg.ID]
	if !ok {
		return sql.ErrNoRows
	}
	m.Parts = arg.Parts
	f.messages[arg.ID] = m
	return nil
}

func (f *fakeQuerier) DeleteMessage(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.messages, id)
	return nil
}

func TestMessageCreatePublishesAndAppendsFinish(t *testing.T) {
	q := newFakeQuerier()
	svc := NewService(q)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := svc.Subscribe(ctx)

	msg, err := svc.Create(context.Background(), "sess-1", CreateMessageParams{
		Role:  User,
		Parts: []ContentPart{TextContent{Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if msg.ID == "" || msg.SessionID != "sess-1" || msg.Role != User {
		t.Fatalf("unexpected message %#v", msg)
	}
	// A non-assistant message gets a synthetic finish part so it is not treated as in-flight.
	if !msg.IsFinished() {
		t.Fatal("user message should be marked finished")
	}

	select {
	case ev := <-events:
		if ev.Type != pubsub.CreatedEvent || ev.Payload.ID != msg.ID {
			t.Fatalf("event = %#v, want created %q", ev, msg.ID)
		}
	default:
		t.Fatal("Create should publish a CreatedEvent")
	}
}

func TestMessageCreateAssistantHasNoSyntheticFinish(t *testing.T) {
	q := newFakeQuerier()
	svc := NewService(q)
	msg, err := svc.Create(context.Background(), "s", CreateMessageParams{
		Role:  Assistant,
		Parts: []ContentPart{TextContent{Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if msg.IsFinished() {
		t.Fatal("assistant message must not be auto-finished")
	}
}

func TestMessageCreatePropagatesError(t *testing.T) {
	q := newFakeQuerier()
	q.err = errors.New("boom")
	svc := NewService(q)
	if _, err := svc.Create(context.Background(), "s", CreateMessageParams{Role: User}); err == nil {
		t.Fatal("expected error")
	}
}

func TestMessageGetListDeleteRoundTrip(t *testing.T) {
	q := newFakeQuerier()
	svc := NewService(q)
	ctx := context.Background()

	m1, err := svc.Create(ctx, "s", CreateMessageParams{Role: Assistant, Parts: []ContentPart{TextContent{Text: "a"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "s", CreateMessageParams{Role: Assistant, Parts: []ContentPart{TextContent{Text: "b"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "other", CreateMessageParams{Role: Assistant}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Get(ctx, m1.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content().Text != "a" {
		t.Errorf("content = %q, want a", got.Content().Text)
	}

	list, err := svc.List(ctx, "s")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d messages, want 2", len(list))
	}

	if err := svc.Delete(ctx, m1.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, m1.ID); err == nil {
		t.Fatal("deleted message should be gone")
	}
}

func TestMessageDeleteSessionMessages(t *testing.T) {
	q := newFakeQuerier()
	svc := NewService(q)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, "s", CreateMessageParams{Role: Assistant}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Create(ctx, "keep", CreateMessageParams{Role: Assistant}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteSessionMessages(ctx, "s"); err != nil {
		t.Fatalf("DeleteSessionMessages: %v", err)
	}
	if list, _ := svc.List(ctx, "s"); len(list) != 0 {
		t.Fatalf("session s has %d messages, want 0", len(list))
	}
	if list, _ := svc.List(ctx, "keep"); len(list) != 1 {
		t.Fatalf("session keep has %d messages, want 1", len(list))
	}
}

func TestMessageUpdate(t *testing.T) {
	q := newFakeQuerier()
	svc := NewService(q)
	ctx := context.Background()

	msg, err := svc.Create(ctx, "s", CreateMessageParams{Role: Assistant, Parts: []ContentPart{TextContent{Text: "old"}}})
	if err != nil {
		t.Fatal(err)
	}
	msg.Parts = []ContentPart{TextContent{Text: "new"}, Finish{Reason: FinishReasonEndTurn, Time: 42}}
	if err := svc.Update(ctx, msg); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.Get(ctx, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content().Text != "new" {
		t.Errorf("content = %q, want new", got.Content().Text)
	}
	if got.FinishReason() != FinishReasonEndTurn {
		t.Errorf("finish = %q", got.FinishReason())
	}
}
