SHELL := /bin/sh
GO ?= go
TEST_DATABASE_URL ?= postgres://testkit:testkit@localhost:55432/testkit?sslmode=disable

.PHONY: help run fmt fmt-check vet test test-integration coverage ci db-up db-down db-init
help:
	@printf '%s\n' 'make test              Fast unit and HTTP tests with race detector' 'make db-up             Disposable PostgreSQL on localhost:55432' 'make test-integration  Unit + real PostgreSQL tests' 'make ci                Format, vet, integration, race and coverage' 'make db-init           Apply demo schema to local Compose database (once)' 'make run               In-memory API at localhost:8080' 'make db-down           Remove disposable database'
run:
	$(GO) run ./cmd/api
fmt:
	$(GO) fmt ./...
fmt-check:
	@test -z "$$($(GO) fmt ./...)" || { echo 'Formatting needed; run make fmt and commit changes'; exit 1; }
vet:
	$(GO) vet ./...
test:
	$(GO) test -race -count=1 ./...
test-integration:
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' $(GO) test -tags=integration -race -count=1 ./...
coverage:
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' $(GO) test -tags=integration -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
ci: fmt-check vet coverage
db-up:
	docker compose up --detach --wait
db-down:
	docker compose down --volumes
db-init:
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U testkit -d testkit < migrations/001_init.sql
