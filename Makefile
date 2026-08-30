.PHONY: build vet fmt check structs test up down migrate run plan storage-check

GO      ?= go
COMPOSE ?= docker compose -f deploy/docker-compose.yml
MYSQL   ?= docker exec -i movie-mysql mysql -h127.0.0.1 -umovie -pmovie movie_streamer

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w catalog-service/

structs:
	@bash scripts/check_structs.sh

check: fmt build vet structs
	@echo "all checks passed"

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

migrate:
	@for f in migrations/*.up.sql; do echo "applying $$f"; $(MYSQL) < $$f; done

run:
	$(GO) run ./catalog-service serve

plan:
	$(GO) run ./catalog-service plan

storage-check:
	$(GO) run ./catalog-service storage-check --write
