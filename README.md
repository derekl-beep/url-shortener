# URL Shortener — System Design Practice

A hands-on implementation of a URL shortener, built to explore every layer of the system design: key generation, caching strategy, analytics pipeline, and production operability. Each component is implemented from scratch rather than abstracted away.

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
| API | `api/` | 8080 | Registration, redirect, and frontend |
| KGS | `kgs/` | 8081 | Key Generation Service |
| Worker | `worker/` | — | Redis Streams consumer → click_events |

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/urls` | Shorten a URL |
| `GET` | `/{key}` | Redirect to original URL |
| `GET` | `/healthz` | Dependency health check |

```bash
curl -X POST localhost:8080/urls \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
# {"short_url":"http://localhost:8080/xxxxxxxx","key":"xxxxxxxx"}

curl -L localhost:8080/<key>
```

## Local Development

**Prerequisites:** Go 1.25+, Docker

```bash
make infra    # start Postgres + Redis
make migrate  # run schema migrations
make seed     # pre-generate key pool (one-time)

# start each service in a separate terminal
make kgs
make api
make worker
```

**Full stack via Docker Compose:**

```bash
# 1. Start infrastructure and run migrations + seed while Postgres is accessible
docker-compose up postgres -d
make migrate
make seed

# 2. Start all services
docker-compose up
```

**Active development (faster iteration, no image rebuild):**

```bash
make infra    # start Postgres + Redis only
make migrate
make seed
make kgs
make api
make worker
```

**Build Docker images manually:**

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
| `CONSUMER_NAME` | hostname | worker |
| `RATE_LIMIT_RPM` | `10` | api |
| `RATE_LIMIT_BURST` | `10` | api |

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

- **KGS pre-generation:** keys are claimed atomically with `FOR UPDATE SKIP LOCKED` — no collision retries on writes
- **Redis on write:** new URLs are cached immediately after DB insert so redirects work before read replicas catch up
- **SHA256 dedup:** duplicate URLs are detected in a single indexed lookup before touching the KGS
- **302 redirect:** preserves click analytics (301 would be cached by browsers indefinitely)
- **Redis Streams:** click events are published after each redirect — non-blocking, outside the critical path. Consumer groups give the same delivery guarantees as Kafka at this scale without the operational overhead.

## What's Built

**Core components**

| Area | Status |
|------|--------|
| DB schema | ✅ |
| KGS | ✅ |
| Registration API | ✅ |
| Redirect API | ✅ |
| Redis caching | ✅ |
| Analytics pipeline (Redis Streams → worker → Postgres) | ✅ |
| Frontend UI | ✅ |

**Production improvements**

| Area | Status |
|------|--------|
| Timeouts | ✅ |
| Graceful shutdown | ✅ |
| Structured logging (`log/slog`) | ✅ |
| Health check (`/healthz` on API + KGS) | ✅ |
| Multi-stage Dockerfiles (`golang:1.22-alpine` → `alpine:3`) | ✅ |
| Worker horizontal scaling (`CONSUMER_NAME`) | ✅ |
| Rate limiting — per-IP token bucket on `POST /urls` (`RATE_LIMIT_RPM`, `RATE_LIMIT_BURST`) | ✅ |
| URL blocklist — rejects loopback, RFC1918, link-local, and cloud metadata endpoints | ✅ |

## Production Gaps

| Area | Item |
|------|------|
| Reliability | KGS circuit breaker — no retry/backoff on KGS failure |
| Reliability | Worker dead-letter queue — failed inserts are dropped, not retried |
| Observability | Prometheus metrics — latency, cache hit rate, stream lag, KGS buffer depth |
| Security | Security headers — CSP, `X-Frame-Options`, `X-Content-Type-Options` |
| Scalability | KGS partitioning — replicas currently claim overlapping key batches |
| Scalability | DB connection pool tuning — `pgxpool` defaults not tuned for production load |
| Scalability | Cache expiry gap — expired URLs can still redirect if cached |
| Operations | Migration tooling — `golang-migrate` or `goose` for versioning and rollback |
| Features | Analytics endpoint — `GET /stats/{key}` to surface `click_events` data |
| Features | Custom aliases — optional vanity slug on `POST /urls` |
| Features | Link management — list/delete owned links (requires auth) |
