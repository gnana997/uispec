# Makefile for uispec

.PHONY: all build test clean install deps fmt lint help docgen-bundle

# Binary name
BINARY := uispec

# Build output directory
BIN_DIR := bin

# Installation prefix
PREFIX ?= /usr/local

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

# Build flags
LDFLAGS := -ldflags "-s -w"

all: docgen-bundle build

## docgen-bundle: Build the docgen worker JS bundle for Node.js enrichment
docgen-bundle:
	@echo "Building docgen worker bundle..."
	@cd scripts && npm install --silent && node build.mjs
	@mkdir -p pkg/scanner/scripts/dist
	@cp scripts/dist/docgen-worker.js pkg/scanner/scripts/dist/docgen-worker.js
	@cp scripts/dist/tokens-worker.js pkg/scanner/scripts/dist/tokens-worker.js
	@echo "Bundles ready (docgen: $(shell du -h pkg/scanner/scripts/dist/docgen-worker.js | cut -f1), tokens: $(shell du -h pkg/scanner/scripts/dist/tokens-worker.js | cut -f1))"

## build: Build uispec binary
build:
	@echo "Building uispec..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) ./cmd/uispec
	@echo "Build complete!"

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...

## test-coverage: Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

## deps: Download and verify dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) verify
	@echo "Dependencies installed!"

## tidy: Tidy go.mod and go.sum
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

## fmt: Format all Go files
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

## lint: Run golangci-lint (requires golangci-lint installed)
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install from https://golangci-lint.run/"; \
	fi

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BIN_DIR)
	rm -f coverage.txt coverage.html
	rm -rf scripts/dist scripts/node_modules
	rm -rf pkg/scanner/scripts/dist
	@echo "Clean complete!"

## install: Install binary to PREFIX/bin
install: build
	@echo "Installing to $(PREFIX)/bin..."
	install -d $(PREFIX)/bin
	install -m 755 $(BIN_DIR)/$(BINARY) $(PREFIX)/bin/
	@echo "Installation complete!"

## uninstall: Remove installed binary
uninstall:
	@echo "Uninstalling from $(PREFIX)/bin..."
	rm -f $(PREFIX)/bin/$(BINARY)
	@echo "Uninstall complete!"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
