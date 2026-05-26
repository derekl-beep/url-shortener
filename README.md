# URL Shortener — System Design Summary

## Core Flow

- User submits a long URL → backend assigns a pre-generated short key → stores mapping in DB → returns `https://<domain>/<short_key>`
- On redirect: backend looks up `short_key` in Redis first, then DB → returns `302` redirect to preserve analytics

---

## Database Schema

```
urls
├── short_key     VARCHAR(8)    PRIMARY KEY
├── original_url  TEXT          NOT NULL
├── url_hash      CHAR(64)      UNIQUE INDEX  ← SHA256 of original_url, for fast dedup
├── created_at    TIMESTAMP
└── expires_at    TIMESTAMP     NULLABLE      ← NULL = never expires
```

Separate `click_events` table (or analytics store) for redirect analytics.

---

## Key Generation Service (KGS)

- Pre-generates a large pool of random short keys, stored in a keys DB (`keys_available` / `keys_used`)
- Multiple replicas, each claims a partitioned batch of keys atomically on startup → no duplicates
- Each replica holds an in-memory buffer of keys — if it crashes, those keys are wasted but correctness is preserved

---

## Scaling

- **Backend:** horizontally scaled behind a load balancer
- **Reads:** Redis cache (check first) + read replicas. Solves replication lag — new URLs are written to Redis immediately on creation
- **Writes:** KGS eliminates expensive hash collision writes. Primary DB handles writes only
- **CDN** layer in front for caching popular redirects at the edge

---

## Analytics Pipeline

- `302` redirect chosen to preserve click data
- Redirect event is logged async to Kafka/SQS (non-blocking, not in the critical path)
- Consumer workers write to a columnar store (ClickHouse / BigQuery) optimized for append-heavy workloads

**Click event schema:**

```
click_events
├── short_key     VARCHAR(8)
├── clicked_at    TIMESTAMP
├── ip_address    VARCHAR
├── user_agent    TEXT
├── referrer      TEXT
└── country       VARCHAR    ← derived from IP via geo lookup
```

---

## Full Architecture

```
Client
  ↓
CDN
  ↓
Load Balancer
  ↓
Backend Servers (horizontal)
  ↓              ↓                ↓
Redis Cache    KGS replicas     Kafka
               (partitioned     (async events)
               key batches)         ↓
  ↓                          Analytics Workers
Primary DB                         ↓
  ↓                          ClickHouse / BigQuery
Read Replicas
```

---

## Recommended Build Order

1. DB schema
2. KGS (key pool + batch claiming)
3. Registration API
4. Redirect API
5. Redis caching layer
6. Analytics pipeline
7. Frontend UI