-- +migrate Up

CREATE TABLE keys_available (
    key_value VARCHAR(8) PRIMARY KEY
);

CREATE TABLE keys_used (
    key_value VARCHAR(8) PRIMARY KEY,
    used_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

