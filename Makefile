BIN     := bin/agent-session-switch
PKG     := ./cmd/agent-session-switch
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test cover lint fmt check install clean

build: ## Build the binary into bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

test: ## Run all tests with the race detector
	go test -race ./...

cover: ## Run tests and print coverage
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

lint: ## Static analysis (needs staticcheck)
	go vet ./...
	staticcheck ./...

fmt: ## Format the code
	gofmt -w .

check: ## Everything CI runs
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go test -race ./...

install: ## Install into $GOBIN
	go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

clean:
	rm -rf bin dist coverage.out
