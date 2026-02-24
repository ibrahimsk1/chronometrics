.PHONY: build test lint docker-up docker-down

build:
	go build ./...

test:
	go test ./...

lint:
	@golangci-lint run || true

docker-up:
	docker compose up -d

docker-down:
	docker compose down

