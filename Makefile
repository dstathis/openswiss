IMAGE  := dstathis/openswiss
TAG    := $(or $(IMAGE_TAG),latest)

## ── Build & Push ───────────────────────────────────────

.PHONY: build push

build: ## Build the Docker image
	docker build -t $(IMAGE):$(TAG) .

push: build ## Build and push the Docker image to Docker Hub
	docker push $(IMAGE):$(TAG)

## ── Local Development ──────────────────────────────────

.PHONY: dev dev-down dev-logs

dev: ## Start all services locally (self-signed TLS on localhost)
	docker compose up -d

dev-down: ## Stop local services
	docker compose down

dev-logs: ## Tail logs from all services
	docker compose logs -f

## ── Production Deploy ──────────────────────────────────

.PHONY: deploy deploy-down deploy-logs

deploy: ## Deploy to production (requires DOMAIN env var)
	@test -n "$(DOMAIN)" || (echo "ERROR: DOMAIN is required, e.g. make deploy DOMAIN=tournaments.example.com" && exit 1)
	DOMAIN=$(DOMAIN) IMAGE_TAG=$(TAG) docker compose up -d --pull always

deploy-down: ## Stop production services
	docker compose down

deploy-logs: ## Tail production logs
	docker compose logs -f

## ── Go Development ─────────────────────────────────────

.PHONY: run test test-integration lint fmt

run: ## Run the server locally (requires DATABASE_URL)
	go run ./cmd/openswiss

test: ## Run unit tests
	go test ./... -count=1

test-integration: ## Run integration tests (requires TEST_DATABASE_URL)
	@test -n "$(TEST_DATABASE_URL)" || (echo "ERROR: TEST_DATABASE_URL is required" && exit 1)
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -tags integration -p 1 ./...

lint: ## Run go vet
	go vet ./...

fmt: ## Format Go source files
	gofmt -w .

## ── Database ───────────────────────────────────────────

.PHONY: db-shell promote-admin

db-shell: ## Open a psql shell to the compose database
	docker compose exec db psql -U openswiss

promote-admin: ## Promote a user to admin (requires EMAIL)
	@test -n "$(EMAIL)" || (echo "ERROR: EMAIL is required, e.g. make promote-admin EMAIL=you@example.com" && exit 1)
	docker compose exec db psql -U openswiss -c \
		"UPDATE users SET roles = '{player,organizer,admin}' WHERE email = '$(EMAIL)';"

## ── Misc ───────────────────────────────────────────────

.PHONY: clean help

clean: ## Remove build artifacts and stopped containers
	rm -f openswiss
	docker compose down --rmi local -v --remove-orphans 2>/dev/null || true

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
