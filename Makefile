.PHONY: up down migrate api worker web test

up:
	docker compose up -d

down:
	docker compose down -v

migrate:
	for f in $(shell ls migrations/*.sql 2>/dev/null | sort); do \
		echo "Applying $$f"; \
		docker compose exec -T postgres psql -U litflow -d litflow < $$f; \
	done

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

web:
	cd apps/web && rm -rf .next && npm install && npm run dev

test:
	go test ./...
