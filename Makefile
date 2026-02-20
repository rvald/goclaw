.PHONY: test test-v lint build run run-gateway
test:                             ## Run all tests
	go test ./...
test-v:                           ## Run tests with verbose output
	go test -v -count=1 ./...
test-cover:                       ## Run tests with coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
lint:                             ## Run golangci-lint
	golangci-lint run
build:                            ## Build binary
	go build -o bin/goclaw ./cmd/goclaw
# Shared logic to generate a token if not provided
TOKEN ?= $$(openssl rand -hex 16)
ARGS = --token "$(TOKEN)" --discord-token "$$DISCORD_TOKEN" --guild-id "$$GUILD_ID"

run: build                        ## Run with Discord (receives secrets from .env + auto-gen token)
	@TOKEN=$${TOKEN:-$$(openssl rand -hex 16)}; \
	echo "Generated Token: $$TOKEN"; \
	set -a && . ./.env && set +a && \
	./bin/goclaw server --token "$$TOKEN" --discord-token "$$DISCORD_TOKEN" --guild-id "$$GUILD_ID"

run-nodiscord: build              ## Run WITHOUT Discord (auto-gen token)
	@TOKEN=$${TOKEN:-$$(openssl rand -hex 16)}; \
	echo "Generated Token: $$TOKEN"; \
	./bin/goclaw server --token "$$TOKEN"

run-remote: build                 ## Run REMOTE with Discord (bind 0.0.0.0:18790)
	@TOKEN=$${TOKEN:-$$(openssl rand -hex 16)}; \
	echo "Generated Token: $$TOKEN"; \
	set -a && . ./.env && set +a && \
	./bin/goclaw server --bind lan --port 18789 --token "$$TOKEN" --discord-token "$$DISCORD_TOKEN" --guild-id "$$GUILD_ID"

run-remote-nodiscord: build       ## Run REMOTE WITHOUT Discord (bind 0.0.0.0:18790)
	@TOKEN=$${TOKEN:-$$(openssl rand -hex 16)}; \
	echo "Generated Token: $$TOKEN"; \
	./bin/goclaw server --bind lan --port 18789 --token "$$TOKEN"
