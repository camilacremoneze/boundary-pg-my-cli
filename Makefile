BINARY     := boundary-pg-my-cli 
MODULE     := github.com/camilacremoneze/pgcli-boundary-vault-integration
BUILD_DIR  := .
GOFLAGS    := -trimpath
LDFLAGS    := -s -w

# Load .env so make targets inherit the same variables as the app
-include .env
export

.PHONY: all build run clean test lint fmt deps docker-build docker-run help

## all: build the binary (default target)
all: build

## build: compile the desktop binary for the current OS/arch
build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) .

## run: build and launch the GUI
run: build
	./$(BINARY)

## test: run all unit tests
test:
	go test ./...

## lint: run go vet and staticcheck (install staticcheck if missing)
lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck ./...

## fmt: format all Go source files
fmt:
	gofmt -w -s .

## deps: download and tidy module dependencies
deps:
	go mod download
	go mod tidy

## clean: remove the compiled binary
clean:
	rm -f $(BUILD_DIR)/$(BINARY)

## docker-build: build the Docker image
docker-build:
	docker compose build

## docker-run: run the app inside Docker (requires X11/XQuartz forwarding)
docker-run:
	docker compose up

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
