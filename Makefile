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
run: build                        ## Run with Discord (sources .env)
	set -a && . ./.env && set +a && \
	./bin/goclaw --token test-secret --discord-token "$$DISCORD_TOKEN" --guild-id "$$GUILD_ID"
run-gateway: build                ## Run gateway only (no Discord)
	./bin/goclaw --token test-secret --port 18789