APP_NAME := gdrive-bisync
DIST_DIR := dist
RELEASE_DIR := release
LOG_DIR := logs
CONFIG_EXAMPLE := config/config.example.json

.PHONY: all clean linux windows release build dev

all: release

dev:
	@echo "Starting development server..."
	go run cmd/gdrive-bisync/main.go $(ARGS)

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