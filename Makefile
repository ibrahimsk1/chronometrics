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

e2e-tests:
	# Start services once (do not tear down per test). Tests will clean data as needed.
	@echo "checking docker compose services and ingestor health..."
	# Check if the ingestor service is running in this compose project.
	@if docker compose ps --services --filter "status=running" 2>/dev/null | grep -qw ingestor; then \
		# If running, verify HTTP health endpoint responds before skipping start. \
		if curl -sSf --max-time 2 http://localhost:8080/health >/dev/null 2>&1; then \
			echo "ingestor running and healthy — skipping docker compose up"; \
		else \
			echo "ingestor running but unhealthy or not ready — running docker compose up -d to ensure correct state"; \
			docker compose up -d; \
		fi; \
	else \
		echo "ingestor not running — bringing up services (docker compose up -d)"; \
		docker compose up -d; \
	fi
	go test ./tests/e2e -run TestE2E_ -v

