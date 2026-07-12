DB_URL := postgres://movie:movie@localhost:5544/movie_streamer?sslmode=disable

.PHONY: up down logs run-upload build test tidy fmt vet migrate-up migrate-down

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

logs:
	docker compose -f deploy/docker-compose.yml logs -f

run-upload:
	go run ./upload-service

build:
	go build ./...

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

migrate-up:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" up

migrate-down:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" down 1
