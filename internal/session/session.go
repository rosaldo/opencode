package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/opencode-ai/opencode/internal/db"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/pubsub"
)

// ErrSessionEmpty is returned by Fork when nothing would be copied: the session
// has no messages, or the boundary message is the first one (Spec 09p).
var ErrSessionEmpty = errors.New("session has no messages to fork")

type Session struct {
	ID               string
	ParentSessionID  string
	Title            string
	MessageCount     int64
	PromptTokens     int64
	CompletionTokens int64
	SummaryMessageID string
	Cost             float64
	CreatedAt        int64
	UpdatedAt        int64
}

type Service interface {
	pubsub.Suscriber[Session]
	Create(ctx context.Context, title string) (Session, error)
	CreateTitleSession(ctx context.Context, parentSessionID string) (Session, error)
	CreateTaskSession(ctx context.Context, toolCallID, parentSessionID, title string) (Session, error)
	Get(ctx context.Context, id string) (Session, error)
	List(ctx context.Context) ([]Session, error)
	Save(ctx context.Context, session Session) (Session, error)
	Delete(ctx context.Context, id string) error
	// Fork copies the messages of sessionID that come strictly before the
	// message identified by before into a new session; an empty before copies
	// the whole session (Spec 09p).
	Fork(ctx context.Context, sessionID string, before string) (Session, error)
}

type service struct {
	*pubsub.Broker[Session]
	q db.Querier
}

func (s *service) Create(ctx context.Context, title string) (Session, error) {
	dbSession, err := s.q.CreateSession(ctx, db.CreateSessionParams{
		ID:    uuid.New().String(),
		Title: title,
	})
	if err != nil {
		return Session{}, err
	}
	session := s.fromDBItem(dbSession)
	s.Publish(pubsub.CreatedEvent, session)
	return session, nil
}

func (s *service) CreateTaskSession(ctx context.Context, toolCallID, parentSessionID, title string) (Session, error) {
	dbSession, err := s.q.CreateSession(ctx, db.CreateSessionParams{
		ID:              toolCallID,
		ParentSessionID: sql.NullString{String: parentSessionID, Valid: true},
		Title:           title,
	})
	if err != nil {
		return Session{}, err
	}
	session := s.fromDBItem(dbSession)
	s.Publish(pubsub.CreatedEvent, session)
	return session, nil
}

func (s *service) CreateTitleSession(ctx context.Context, parentSessionID string) (Session, error) {
	dbSession, err := s.q.CreateSession(ctx, db.CreateSessionParams{
		ID:              "title-" + parentSessionID,
		ParentSessionID: sql.NullString{String: parentSessionID, Valid: true},
		Title:           "Generate a title",
	})
	if err != nil {
		return Session{}, err
	}
	session := s.fromDBItem(dbSession)
	s.Publish(pubsub.CreatedEvent, session)
	return session, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	session, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	err = s.q.DeleteSession(ctx, session.ID)
	if err != nil {
		return err
	}
	s.Publish(pubsub.DeletedEvent, session)
	return nil
}

// Fork creates a new session holding a copy of the messages of sessionID that
// come strictly before the message identified by before (Spec 09p). An empty
// before copies the whole session. Assistant messages still in flight (no
// Finish yet) are skipped, the copy keeps the original timestamps, and the
// origin is recorded in fork_session_id — not parent_session_id — so the fork
// keeps showing up in List.
func (s *service) Fork(ctx context.Context, sessionID string, before string) (Session, error) {
	parent, err := s.Get(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	parentMsgs, err := s.q.ListMessagesBySession(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	limit := len(parentMsgs)
	if before != "" {
		limit = -1
		for i, m := range parentMsgs {
			if m.ID == before {
				limit = i
				break
			}
		}
		if limit < 0 {
			return Session{}, fmt.Errorf("message %s not found in session %s", before, sessionID)
		}
	}

	copied := make([]db.Message, 0, limit)
	for _, m := range parentMsgs[:limit] {
		if messageSettled(m) {
			copied = append(copied, m)
		}
	}
	if len(copied) == 0 {
		return Session{}, ErrSessionEmpty
	}

	forkN, err := s.q.CountSessionForks(ctx, sql.NullString{String: sessionID, Valid: true})
	if err != nil {
		return Session{}, err
	}
	dbSession, err := s.q.CreateForkSession(ctx, db.CreateForkSessionParams{
		ID:            uuid.New().String(),
		ForkSessionID: sql.NullString{String: sessionID, Valid: true},
		Title:         fmt.Sprintf("%s (fork #%d)", parent.Title, forkN+1),
	})
	if err != nil {
		return Session{}, err
	}
	for _, m := range copied {
		if _, err := s.q.CopyMessage(ctx, db.CopyMessageParams{
			ID:         uuid.New().String(),
			SessionID:  dbSession.ID,
			Role:       m.Role,
			Parts:      m.Parts,
			Model:      m.Model,
			CreatedAt:  m.CreatedAt,
			UpdatedAt:  m.UpdatedAt,
			FinishedAt: m.FinishedAt,
		}); err != nil {
			// A partial transcript must not survive (Spec 09p).
			if delErr := s.q.DeleteSession(ctx, dbSession.ID); delErr != nil {
				return Session{}, errors.Join(err, delErr)
			}
			return Session{}, err
		}
	}
	session := s.fromDBItem(dbSession)
	s.Publish(pubsub.CreatedEvent, session)
	return session, nil
}

// messageSettled reports whether a stored row is complete: non-assistant rows
// are created with their Finish part already appended, and an assistant row
// only gets finished_at once its Finish was persisted by Update (Spec 09p).
func messageSettled(m db.Message) bool {
	return m.Role != string(message.Assistant) || m.FinishedAt.Valid
}

func (s *service) Get(ctx context.Context, id string) (Session, error) {
	dbSession, err := s.q.GetSessionByID(ctx, id)
	if err != nil {
		return Session{}, err
	}
	return s.fromDBItem(dbSession), nil
}

func (s *service) Save(ctx context.Context, session Session) (Session, error) {
	dbSession, err := s.q.UpdateSession(ctx, db.UpdateSessionParams{
		ID:               session.ID,
		Title:            session.Title,
		PromptTokens:     session.PromptTokens,
		CompletionTokens: session.CompletionTokens,
		SummaryMessageID: sql.NullString{
			String: session.SummaryMessageID,
			Valid:  session.SummaryMessageID != "",
		},
		Cost: session.Cost,
	})
	if err != nil {
		return Session{}, err
	}
	session = s.fromDBItem(dbSession)
	s.Publish(pubsub.UpdatedEvent, session)
	return session, nil
}

func (s *service) List(ctx context.Context) ([]Session, error) {
	dbSessions, err := s.q.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, len(dbSessions))
	for i, dbSession := range dbSessions {
		sessions[i] = s.fromDBItem(dbSession)
	}
	return sessions, nil
}

func (s service) fromDBItem(item db.Session) Session {
	return Session{
		ID:               item.ID,
		ParentSessionID:  item.ParentSessionID.String,
		Title:            item.Title,
		MessageCount:     item.MessageCount,
		PromptTokens:     item.PromptTokens,
		CompletionTokens: item.CompletionTokens,
		SummaryMessageID: item.SummaryMessageID.String,
		Cost:             item.Cost,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}

func NewService(q db.Querier) Service {
	broker := pubsub.NewBroker[Session]()
	return &service{
		broker,
		q,
	}
}
