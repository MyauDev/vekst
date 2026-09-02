-- NOT a migration. goose creates this table itself on first run; it is not
-- created by any file in core/migrations, so sqlc -- which only reads DDL,
-- never runs it -- has nothing to learn its shape from. This file exists
-- solely so sqlc can type-check core/internal/db/query/readiness.sql against
-- goose's actual column set. It must never be passed to `goose up`.
CREATE TABLE goose_db_version (
    id bigserial PRIMARY KEY,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp NOT NULL DEFAULT now()
);
