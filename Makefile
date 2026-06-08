.PHONY: build test lint migrate dev clean

build:
	go build -o bin/flowctl-server ./cmd/flowctl-server
	go build -o bin/flowctl-worker ./cmd/flowctl-worker
	go build -o bin/flowctl ./cmd/flowctl

test:
	go test ./... -race -count=1

test-integration:
	go test ./... -tags=integration -race

lint:
	golangci-lint run ./...

migrate-up:
	goose -dir internal/repository/migrations postgres "$$DATABASE_URL" up

migrate-down:
	goose -dir internal/repository/migrations postgres "$$DATABASE_URL" down

migrate-create:
	goose -dir internal/repository/migrations create $(NAME) sql

dev:
	docker compose -f deploy/docker-compose.yml up --build

clean:
	rm -rf bin/ dist/

server:
	go run ./cmd/flowctl-server

worker:
	go run ./cmd/flowctl-worker
