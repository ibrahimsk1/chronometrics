.PHONY: build test lint docker-up docker-down test-contract

build:
	go build ./...

test:
	go test ./...

lint:
	@golangci-lint run || true
 
lint-openapi:
	# placeholder for OpenAPI linting (e.g., redocly/openapi-cli)
	@echo "lint-openapi (placeholder)"

test-contract:
	go test ./tests/contract -v

docker-up:
	docker compose up -d

docker-down:
	docker compose down

