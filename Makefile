APP_NAME  := eve-flipper
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DIR := build
LDFLAGS   := -s -w -X main.version=$(VERSION)

.PHONY: all build run test clean frontend frontend-wails wails wails-run cross

## build: build frontend + backend into a single binary
build: frontend
	@mkdir -p $(BUILD_DIR)
	set -a; \
	if [ -f .env ]; then \
		tr -d '\r' < .env > .env.__tmp; \
		. ./.env.__tmp; \
		rm -f .env.__tmp; \
	fi; \
	set +a; \
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) .

## run: build and run
run: build
	$(BUILD_DIR)/$(APP_NAME)

## test: run all Go tests
test:
	go test ./...

## frontend: install deps and build frontend
frontend:
	cd frontend && npm install && npm run build

## frontend-wails: install deps and build frontend for Wails (API -> 127.0.0.1:13370)
frontend-wails:
	cd frontend && npm install && npm run build:wails

## wails: build Wails desktop variant (single-process UI + backend API)
wails: frontend-wails
	@mkdir -p $(BUILD_DIR)
	go build -tags wails,production -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-wails .

## wails-run: build and run Wails desktop variant
wails-run: wails
	$(BUILD_DIR)/$(APP_NAME)-wails

## cross: build release binaries for Windows, Linux, macOS (amd64 + arm64)
cross: frontend
	@mkdir -p $(BUILD_DIR)
	set -a; \
	if [ -f .env ]; then \
		tr -d '\r' < .env > .env.__tmp; \
		. ./.env.__tmp; \
		rm -f .env.__tmp; \
	fi; \
	set +a; \
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe .; \
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .; \
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 .; \
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 .; \
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 .
	@echo "Binaries in $(BUILD_DIR)/"

## clean: remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## all: test + cross-compile (includes frontend)
all: test cross
