DB_URL=postgres://postgres:postgres@localhost:5432/urlshortener

.PHONY: infra migrate seed kgs api worker

infra:
	docker-compose up -d postgres redis

migrate:
	docker exec -i url-shortener-postgres-1 psql -U postgres -d urlshortener < migrations/001_initial_schema.sql
	docker exec -i url-shortener-postgres-1 psql -U postgres -d urlshortener < migrations/002_keys_schema.sql

seed:
	SEED_COUNT=1000000 DATABASE_URL=$(DB_URL) go run ./kgs

kgs:
	DATABASE_URL=$(DB_URL) go run ./kgs

api:
	DATABASE_URL=$(DB_URL) KGS_URL=http://localhost:8081 REDIS_ADDR=localhost:6379 go run ./api

worker:
	DATABASE_URL=$(DB_URL) REDIS_ADDR=localhost:6379 go run ./worker
