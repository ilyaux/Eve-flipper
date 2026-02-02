APP_NAME  := eve-flipper
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DIR := build
LDFLAGS   := -s -w -X main.version=$(VERSION)

.PHONY: all build run test clean frontend cross

## build: build frontend + backend into a single binary
build: frontend
	@mkdir -p $(BUILD_DIR)
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

## cross: build release binaries for Windows, Linux, macOS (amd64 + arm64)
cross: frontend
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe .
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 .
	@echo "Binaries in $(BUILD_DIR)/"

## clean: remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## all: test + cross-compile (includes frontend)
all: test cross
