# Makefile for gorestic-homelab
# Run 'make help' to see available targets

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=gofmt
GOMOD=$(GOCMD) mod
BINARY_NAME=gorestic-homelab
MAIN_PACKAGE=./cmd/gorestic-homelab

# Build flags
LDFLAGS=-ldflags "-s -w"

# Test flags
TEST_FLAGS=-v -race
COVERAGE_FILE=coverage.out

.PHONY: all build clean test test-unit test-integration test-e2e test-all lint fmt vet deps help

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/ /'

## all: Run fmt, lint, vet, test and build
all: fmt lint vet test build

## build: Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

## clean: Remove build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f $(COVERAGE_FILE)
	$(GOCMD) clean

## deps: Download dependencies
deps:
	$(GOMOD) download

## deps-tidy: Tidy dependencies
deps-tidy:
	$(GOMOD) tidy

## fmt: Format code
fmt:
	$(GOFMT) -s -w .

## fmt-check: Check code formatting
fmt-check:
	@if [ "$$($(GOFMT) -s -l . | wc -l)" -gt 0 ]; then \
		echo "Code is not formatted:"; \
		$(GOFMT) -s -l .; \
		exit 1; \
	fi

## vet: Run go vet
vet:
	$(GOVET) ./...

## lint: Run golangci-lint
lint:
	golangci-lint run

## test: Run unit tests (alias for test-unit)
test: test-unit

## test-unit: Run unit tests only (excludes integration and e2e)
test-unit:
	$(GOTEST) $(TEST_FLAGS) ./...

## test-unit-cover: Run unit tests with coverage
test-unit-cover:
	$(GOTEST) $(TEST_FLAGS) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...

## test-integration: Run integration tests only
test-integration:
	$(GOTEST) $(TEST_FLAGS) -tags=integration ./integration/...

## test-e2e: Run e2e tests only
test-e2e:
	$(GOTEST) $(TEST_FLAGS) -tags=e2e ./e2e/...

## test-all: Run all tests (unit, integration, and e2e)
test-all:
	$(GOTEST) $(TEST_FLAGS) -tags=integration,e2e ./...

## cover: Generate coverage report
cover: test-unit-cover
	$(GOCMD) tool cover -html=$(COVERAGE_FILE)

## run: Run the application
run: build
	./$(BINARY_NAME)

## docker-build: Build Docker image
docker-build:
	docker build -t $(BINARY_NAME):latest .

## mockery: Generate mocks
mockery:
	mockery
