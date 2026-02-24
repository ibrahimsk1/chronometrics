SHELL := /bin/bash
export PATH := /usr/local/go/bin:/opt/homebrew/bin:/usr/local/bin:$(PATH)

E2E_REPORT_DIR ?= reports/e2e
E2E_TIMEOUT    ?= 120s

.PHONY: build test lint docker-up docker-down test-contract e2e e2e-tests

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

## Run E2E tests against the docker-compose stack and save a report.
## Reports are written to $(E2E_REPORT_DIR)/ (text log + JSON).
## Usage: make e2e
##        make e2e E2E_TIMEOUT=180s
e2e:
	@mkdir -p $(E2E_REPORT_DIR)
	docker compose build ingestor
	docker compose up -d --force-recreate
	@echo "Waiting for ingestor /health to be ready (up to 30s)..."
	@for i in $$(seq 1 30); do \
		curl -sf http://localhost:8080/health > /dev/null 2>&1 && echo "  ready after $${i}s" && break; \
		sleep 1; \
		if [ $$i -eq 30 ]; then echo "ERROR: ingestor not ready after 30s"; docker compose logs ingestor; exit 1; fi; \
	done
	@REPORT=$(E2E_REPORT_DIR)/run_$$(date +%Y%m%d_%H%M%S).log; \
	go test ./tests/e2e/ -run TestE2E_ -v -count=1 -timeout $(E2E_TIMEOUT) \
		2>&1 | tee "$$REPORT"; \
	EXIT=$${PIPESTATUS[0]}; \
	echo ""; \
	echo "------------------------------------------------------------"; \
	if [ $$EXIT -eq 0 ]; then echo "E2E PASSED  — report: $$REPORT"; \
	else echo "E2E FAILED  — report: $$REPORT"; fi; \
	echo "------------------------------------------------------------"; \
	go test ./tests/e2e/ -run TestE2E_ -count=1 -timeout $(E2E_TIMEOUT) -json \
		> $(E2E_REPORT_DIR)/results.json 2>&1 || true; \
	echo "JSON report : $(E2E_REPORT_DIR)/results.json"; \
	exit $$EXIT

e2e-tests: e2e  ## Alias kept for backwards compatibility.

