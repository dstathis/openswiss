IMAGE  := dstathis/openswiss
TAG    := $(or $(IMAGE_TAG),latest)

## ── Build & Push ───────────────────────────────────────

.PHONY: build push pull

build: ## Build the Docker image
	docker build -t $(IMAGE):$(TAG) .

push: build ## Build and push the Docker image to Docker Hub
	docker push $(IMAGE):$(TAG)

pull: ## Pull the Docker image from Docker Hub
	docker pull $(IMAGE):$(TAG)

## ── Local Development ──────────────────────────────────

.PHONY: setup dev dev-down dev-logs

setup: ## Create .env with a generated POSTGRES_PASSWORD (first-time setup)
	@test ! -f .env || (echo "ERROR: .env already exists; refusing to overwrite" && exit 1)
	@cp .env.example .env
	@PASS=$$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32); \
		sed -i "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$$PASS|" .env
	@echo "Wrote .env with a generated POSTGRES_PASSWORD. Edit it to add SMTP / DOMAIN before running 'make dev' or 'make deploy'."

dev: ## Start all services locally (self-signed TLS on localhost)
	@test -f .env || (echo "ERROR: .env not found. Run 'make setup' first." && exit 1)
	docker compose up -d

dev-down: ## Stop local services
	docker compose down

dev-logs: ## Tail logs from all services
	docker compose logs -f

## ── Production Deploy ──────────────────────────────────

.PHONY: deploy deploy-down deploy-logs

deploy: ## Deploy to production (set DOMAIN in .env)
	@test -f .env || (echo "ERROR: .env not found. Run 'make setup' first." && exit 1)
	docker compose up -d --pull always

deploy-down: ## Stop production services
	docker compose down

deploy-logs: ## Tail production logs
	docker compose logs -f

## ── Go Development ─────────────────────────────────────

TEST_DB_CONTAINER := openswiss-test-db
TEST_DB_USER      := openswiss_test
TEST_DB_PASS      := openswiss_test
TEST_DB_NAME      := openswiss_test
TEST_DB_PORT      := 5433
TEST_DATABASE_URL := postgres://$(TEST_DB_USER):$(TEST_DB_PASS)@localhost:$(TEST_DB_PORT)/$(TEST_DB_NAME)?sslmode=disable

.PHONY: run migrate test test-integration test-load test-db-up test-db-down lint fmt

run: ## Run the server locally (requires DATABASE_URL; run `make migrate` first)
	go run . serve

migrate: ## Apply database migrations locally (requires DATABASE_URL)
	go run . migrate

test: ## Run unit tests
	go test ./... -count=1

test-db-up:
	@docker start $(TEST_DB_CONTAINER) 2>/dev/null || \
		docker run -d --name $(TEST_DB_CONTAINER) \
			-e POSTGRES_USER=$(TEST_DB_USER) \
			-e POSTGRES_PASSWORD=$(TEST_DB_PASS) \
			-e POSTGRES_DB=$(TEST_DB_NAME) \
			-p $(TEST_DB_PORT):5432 \
			postgres:18
	@until docker exec $(TEST_DB_CONTAINER) pg_isready -U $(TEST_DB_USER) -q 2>/dev/null; do sleep 0.2; done

test-db-down:
	@docker rm -f $(TEST_DB_CONTAINER) 2>/dev/null || true

test-integration: test-db-up ## Run integration tests (auto-creates test DB)
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -tags integration -p 1 ./...; \
	status=$$?; $(MAKE) test-db-down; exit $$status

test-load: test-db-up ## Run 5000-player load test (auto-creates test DB)
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -tags loadtest -v -timeout 10m ./internal/engine/; \
	status=$$?; $(MAKE) test-db-down; exit $$status

lint: ## Run go vet
	go vet ./...

fmt: ## Format Go source files
	gofmt -w .

## ── Database ───────────────────────────────────────────

.PHONY: db-shell promote-admin verify-user

PG_USER := $(or $(POSTGRES_USER),openswiss)

db-shell: ## Open a psql shell to the compose database
	docker compose exec db psql -U $(PG_USER)

promote-admin: ## Promote a user to admin (requires EMAIL)
	@test -n "$(EMAIL)" || (echo "ERROR: EMAIL is required, e.g. make promote-admin EMAIL=you@example.com" && exit 1)
	docker compose exec db psql -U $(PG_USER) -c \
		"UPDATE users SET roles = '{player,organizer,admin}' WHERE email = '$(EMAIL)';"

verify-user: ## Force a user's email to verified (requires EMAIL)
	@test -n "$(EMAIL)" || (echo "ERROR: EMAIL is required" && exit 1)
	docker compose exec db psql -U $(PG_USER) -c \
		"UPDATE users SET email_verified_at = now() WHERE email = '$(EMAIL)';"

## ── Misc ───────────────────────────────────────────────

.PHONY: clean help

clean: ## Remove build artifacts and stopped containers
	rm -f openswiss
	docker compose down --rmi local -v --remove-orphans 2>/dev/null || true

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
