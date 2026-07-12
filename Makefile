.PHONY: up down logs run-upload build test tidy fmt vet

# --- infra (docker-compose) ---
up:            ## start local infra (Postgres + MinIO + RabbitMQ)
	docker compose -f deploy/docker-compose.yml up -d

down:          ## stop local infra
	docker compose -f deploy/docker-compose.yml down

logs:          ## tail infra logs
	docker compose -f deploy/docker-compose.yml logs -f

# --- services ---
run-upload:    ## run the upload-service on the host
	go run ./cmd/upload-service

# --- go ---
build:         ## compile everything
	go build ./...

test:          ## run tests
	go test ./...

tidy:          ## sync go.mod/go.sum
	go mod tidy

fmt:           ## format
	go fmt ./...

vet:           ## static checks
	go vet ./...
