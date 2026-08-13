APP_NAME := gdrive-bisync
DIST_DIR := dist
RELEASE_DIR := release
LOG_DIR := logs
CONFIG_EXAMPLE := config/config.example.json
GO_CACHE_DIR ?= /tmp/gocache

.PHONY: all clean linux windows release build dev test race vet lint format-check mod-check build-check check

all: release

dev:
	@echo "Starting development server..."
	go run cmd/gdrive-bisync/main.go $(ARGS)

test:
	@echo "Running tests..."
	GOCACHE=$(GO_CACHE_DIR) go test ./... -count=1

race:
	@echo "Running race detector..."
	GOCACHE=$(GO_CACHE_DIR) go test -race ./... -count=1

vet:
	@echo "Running go vet..."
	GOCACHE=$(GO_CACHE_DIR) go vet ./...

lint:
	@echo "Running golangci-lint..."
	golangci-lint run ./...

format-check:
	@echo "Checking Go formatting..."
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))" || { \
		echo "These files need gofmt:"; \
		gofmt -l $$(git ls-files '*.go'); \
		exit 1; \
	}

mod-check:
	@echo "Verifying Go modules..."
	go mod verify
	go mod tidy -diff

build-check:
	@echo "Building Linux binary..."
	GOCACHE=$(GO_CACHE_DIR) go build -trimpath -o /tmp/gdrive-bisync-check ./cmd/gdrive-bisync
	@echo "Building Windows binary..."
	GOCACHE=$(GO_CACHE_DIR) GOOS=windows GOARCH=amd64 go build -trimpath -o /tmp/gdrive-bisync-check.exe ./cmd/gdrive-bisync

check: format-check mod-check lint test race vet build-check
	@echo "All checks passed."

linux:
	@echo "Building for Linux (amd64)..."
	@mkdir -p $(DIST_DIR)/linux/config
	GOOS=linux GOARCH=amd64 go build -o $(DIST_DIR)/linux/$(APP_NAME) cmd/gdrive-bisync/main.go
	@cp $(CONFIG_EXAMPLE) $(DIST_DIR)/linux/config/
	@echo "Linux build ready in $(DIST_DIR)/linux"

windows:
	@echo "Building for Windows (amd64)..."
	@mkdir -p $(DIST_DIR)/windows/config
	GOOS=windows GOARCH=amd64 go build -o $(DIST_DIR)/windows/$(APP_NAME).exe cmd/gdrive-bisync/main.go
	@cp $(CONFIG_EXAMPLE) $(DIST_DIR)/windows/config/
	@echo "Windows build ready in $(DIST_DIR)/windows"

build: linux windows

release: build
	@echo "Creating release archives..."
	@mkdir -p $(RELEASE_DIR)
	@if command -v zip >/dev/null 2>&1; then \
		cd $(DIST_DIR)/linux && zip -r ../../$(RELEASE_DIR)/linux.zip . && cd ../..; \
		cd $(DIST_DIR)/windows && zip -r ../../$(RELEASE_DIR)/windows.zip . && cd ../..; \
		echo "Release archives created in $(RELEASE_DIR)"; \
	else \
		echo "'zip' command not found. Archives skipped."; \
	fi

clean:
	@echo "Cleaning up..."
	rm -rf $(DIST_DIR) $(RELEASE_DIR) $(LOG_DIR)
