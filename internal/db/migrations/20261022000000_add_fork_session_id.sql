-- +goose Up
-- +goose StatementBegin
-- Fork sessions record their origin without becoming task children, so they
-- keep showing up in ListSessions (which filters parent_session_id) (Spec 09p).
ALTER TABLE sessions ADD COLUMN fork_session_id TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sessions DROP COLUMN fork_session_id;
-- +goose StatementEnd
