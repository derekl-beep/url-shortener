-- +migrate Up

CREATE TABLE urls (
    short_key    VARCHAR(8)   PRIMARY KEY,
    original_url TEXT         NOT NULL,
    url_hash     CHAR(64)     NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ  NULL
);

CREATE UNIQUE INDEX idx_urls_url_hash ON urls (url_hash);


CREATE TABLE click_events (
    id           BIGSERIAL    PRIMARY KEY,
    short_key    VARCHAR(8)   NOT NULL REFERENCES urls (short_key),
    clicked_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    ip_address   INET         NULL,
    user_agent   TEXT         NULL,
    referrer     TEXT         NULL,
    country      VARCHAR(2)   NULL
);

CREATE INDEX idx_click_events_short_key ON click_events (short_key);
CREATE INDEX idx_click_events_clicked_at ON click_events (clicked_at);


