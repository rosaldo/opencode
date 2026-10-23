-- +goose Up
-- +goose StatementBegin
-- Per-session working directory (Spec 09v): empty means "use the launch
-- directory" (config.WorkingDirectory()).
ALTER TABLE sessions ADD COLUMN working_directory TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sessions DROP COLUMN working_directory;
-- +goose StatementEnd
