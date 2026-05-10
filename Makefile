SHELL := /bin/sh

GO ?= go
GOCACHE ?= /tmp/go-build
COMPOSE ?= docker compose

SERVICES := postgres kafka microservice2 microservice1

.PHONY: help fmt test build check compose-config up down producer logs-ms1 logs-ms2 logs-kafka stop-ms2 start-ms2

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "%-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

fmt: ## Format Go source files
	@find cmd internal -type f -name '*.go' -print0 | xargs -0 gofmt -w

test: ## Run the Go test suite
	@GOCACHE=$(GOCACHE) $(GO) test ./...

build: ## Build all Go packages
	@GOCACHE=$(GOCACHE) $(GO) build ./...

check: test build compose-config ## Run the main verification checks

compose-config: ## Validate docker compose configuration
	@$(COMPOSE) config >/dev/null
	@echo "docker compose config is valid"

up: ## Build and start the application stack
	@$(COMPOSE) up -d --build $(SERVICES)

down: ## Stop and remove the application stack
	@$(COMPOSE) down

producer: ## Publish test messages with the producer container
	@$(COMPOSE) --profile test run --rm producer

logs-ms1: ## Follow microservice1 logs
	@$(COMPOSE) logs -f microservice1

logs-ms2: ## Follow microservice2 logs
	@$(COMPOSE) logs -f microservice2

logs-kafka: ## Follow kafka logs
	@$(COMPOSE) logs -f kafka

stop-ms2: ## Stop microservice2 for retry testing
	@$(COMPOSE) stop microservice2

start-ms2: ## Start microservice2 after retry testing
	@$(COMPOSE) start microservice2
