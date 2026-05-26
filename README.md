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

**Build Docker images:**

```bash
docker build -f Dockerfile.api    -t url-shortener-api    .
docker build -f Dockerfile.kgs    -t url-shortener-kgs    .
docker build -f Dockerfile.worker -t url-shortener-worker .
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
- [x] Frontend UI

## Production Gaps

Items not yet implemented that a real production deployment would require.

### Reliability
- [ ] **Graceful shutdown** — signal handling + request draining on all three services; currently a SIGTERM drops in-flight requests
- [ ] **Timeouts** — no deadlines on outbound KGS HTTP calls, DB queries, or Redis ops; a slow dependency hangs the goroutine indefinitely
- [ ] **KGS circuit breaker** — `CreateURL` returns 503 immediately on KGS failure with no retry or backoff
- [ ] **Worker dead-letter queue** — failed `click_events` inserts are logged and dropped; redelivery only happens on worker restart

### Observability
- [ ] **Structured logging** — replace `log.Printf` with `log/slog`; unstructured output is hard to query in production
- [ ] **Prometheus metrics** — request latency, cache hit/miss ratio, Redis stream consumer lag, KGS buffer depth
- [ ] **API health endpoint** — `/healthz` exists on KGS but not on the API

### Security
- [ ] **Rate limiting** — no per-IP throttle on `POST /urls` or `GET /{key}`
- [ ] **URL blocklist** — `POST /urls` should reject `localhost`, RFC1918 addresses, and known malicious domains
- [ ] **Security headers** — CSP, `X-Frame-Options`, `X-Content-Type-Options` missing from all responses

### Scalability
- [ ] **KGS partitioning** — multiple KGS replicas currently claim overlapping key batches; each replica needs a distinct partition range
- [ ] **Parameterized worker consumer name** — hardcoded `"worker-1"` prevents horizontal scaling; should be an env var
- [ ] **DB connection pool tuning** — `pgxpool` defaults need explicit `MaxConns`, `MinConns`, `MaxConnLifetime` for production load
- [ ] **Cache expiry for expired URLs** — `store.FindByKey` enforces `expires_at` in SQL, but a cached entry can still redirect after the URL expires

### Operations
- [x] **Multi-stage Dockerfiles** — `golang:1.22-alpine` build stage, `distroless/static` runtime (~2MB image, no shell)
- [ ] **Migration tooling** — raw `psql` pipe works locally; `golang-migrate` or `goose` adds versioning, rollback, and CI integration

### Features
- [ ] **Analytics endpoint** — `click_events` is populated; a `GET /stats/{key}` endpoint would surface the pipeline data
- [ ] **Custom aliases** — allow `POST /urls` to accept an optional `alias` field for vanity URLs
- [ ] **Link management** — list and delete owned links (requires auth)
