# Makefile for GoTodo
# Usage: run `make <target>`

.PHONY: help tidy build run run-api test lint swag swag-run air install-air docker-build docker-run fmt clean

## help: Show this help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## ' Makefile | sort | awk 'BEGIN {FS":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

## tidy: Download and tidy modules
tidy:
	go mod tidy

## build: Build API binary
build: tidy
	go build -o bin/gotodo ./cmd/api

## run: Run API directly (no hot reload)
run:
	go run ./cmd/api

## run-api: Alias for run
run-api: run ## Run the API server

## test: Run tests (if any)
test:
	go test ./...

## lint: Run basic static checks (go vet)
lint:
	go vet ./...

## fmt: Format code
fmt:
	go fmt ./...

## swag: Generate Swagger docs (requires `swag` installed)
swag:
	$$(go env GOPATH)/bin/swag init -g cmd/api/main.go

## swag-run: Generate Swagger docs then run
swag-run: swag run ## Generate Swagger docs then run server

## install-air: Install Air live-reload tool
install-air:
	go install github.com/air-verse/air@latest

## air: Run with hot-reload (recommended for development)
air:
	$$(go env GOPATH)/bin/air -c .air.toml

## docker-build: Build Docker image
docker-build:
	docker build -t gotodo:latest .

## docker-run: Run Docker container on port 8080
docker-run:
	docker run --rm -p 8080:8080 --env-file .env gotodo:latest

## clean: Remove build artifacts
clean:
	rm -rf bin

