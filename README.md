# URL Shortener

A production-grade URL shortener built to explore every component of the system design.

## Architecture

```
Client
  ↓
CDN
  ↓
Load Balancer
  ↓
Backend Servers (horizontal)
  ↓              ↓                ↓
Redis Cache    KGS replicas     Redis Streams
               (partitioned     (async click events)
               key batches)         ↓
  ↓                          Analytics Workers
Primary DB                         ↓
  ↓                          click_events (Postgres)
Read Replicas
```

## Services

| Service | Directory | Port | Description |
|---------|-----------|------|-------------|
| API | `api/` | 8080 | Registration and redirect endpoints |
| KGS | `kgs/` | 8081 | Key Generation Service |
| Worker | `worker/` | — | Redis Streams consumer, writes click events to DB |

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/urls` | Shorten a URL |
| `GET` | `/{key}` | Redirect to original URL |

**Shorten a URL:**
```bash
curl -X POST localhost:8080/urls \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

# {"short_url":"http://localhost:8080/xxxxxxxx","key":"xxxxxxxx"}
```

**Redirect:**
```bash
curl -L localhost:8080/<key>
```

## Local Development

**Prerequisites:** Go 1.22+, Docker

```bash
# 1. Start infrastructure (Postgres + Redis)
make infra

# 2. Run migrations
make migrate

# 3. Seed the key pool (one-time)
make seed

# 4. Start services in separate terminals
make kgs
make api
make worker
```

**Environment variables:**

| Variable | Default | Service |
|----------|---------|---------|
| `DATABASE_URL` | — | api, kgs |
| `KGS_URL` | — | api |
| `REDIS_ADDR` | — | api, worker |
| `BASE_URL` | `http://localhost:8080` | api |
| `PORT` | `8080` / `8081` | api / kgs |
| `BATCH_SIZE` | `10000` | kgs |
| `REFILL_THRESHOLD` | `1000` | kgs |
| `SEED_COUNT` | `0` | kgs |

## Database Schema

```
urls
├── short_key     VARCHAR(8)    PRIMARY KEY
├── original_url  TEXT          NOT NULL
├── url_hash      CHAR(64)      UNIQUE INDEX  ← SHA256 of original_url, for dedup
├── created_at    TIMESTAMPTZ
└── expires_at    TIMESTAMPTZ   NULLABLE      ← NULL = never expires

keys_available
└── key_value     VARCHAR(8)    PRIMARY KEY   ← pre-generated pool

keys_used
├── key_value     VARCHAR(8)    PRIMARY KEY   ← audit trail
└── used_at       TIMESTAMPTZ

click_events                                  ← populated by analytics pipeline
├── short_key     VARCHAR(8)
├── clicked_at    TIMESTAMPTZ
├── ip_address    INET
├── user_agent    TEXT
├── referrer      TEXT
└── country       VARCHAR(2)    ← derived from IP via geo lookup
```

## Key Design Decisions

- **KGS pre-generation:** keys are generated in bulk and claimed atomically with `FOR UPDATE SKIP LOCKED` — no collision retries on writes
- **Redis on write:** new URLs are written to Redis immediately after DB insert, so redirects work before read replicas catch up
- **SHA256 dedup:** duplicate URLs are detected in a single indexed lookup before touching the KGS
- **302 redirect:** preserves click analytics (301 would be cached by browsers indefinitely)
- **Redis Streams analytics:** click events are published to a Redis Stream after each redirect — non-blocking, outside the critical path. Consumer groups give the same delivery guarantees as Kafka at this scale without the operational overhead.

## Build Progress

- [x] DB schema
- [x] KGS
- [x] Registration API
- [x] Redirect API
- [x] Redis caching
- [x] Analytics pipeline (Redis Streams → worker → Postgres)
- [ ] Frontend UI
