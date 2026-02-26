# MyDiscoverer build and code generation.

.PHONY: install generate test build help test-redis

help:
	@echo "MyDiscoverer Makefile"
	@echo ""
	@echo "  make generate  - Generate code from OpenAPI and mocks"
	@echo "  make test      - Run all tests"
	@echo "  make build     - Build mydiscoverer binary"
	@echo "  make test-redis - Run Redis adapter tests with docker-compose"
	@echo ""

generate:
	@echo "+++ MyDiscoverer: Generating code..."
	@go generate ./...
	@echo "+++ MyDiscoverer: Code generation complete"

test:
	@echo "+++ MyDiscoverer: Running tests..."
	@go test -v ./...
	@echo "+++ MyDiscoverer: Tests completed"

build:
	@echo "+++ MyDiscoverer: Building..."
	@go build -ldflags="-s -w" -o mydiscoverer ./cmd
	@echo "+++ MyDiscoverer: Build complete"

test-redis:
	@echo "Starting Redis test container..."
	@docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for Redis..."
	@sleep 2
	@go test -v ./adapters/myredis/... || (docker-compose -f docker-compose.test.yml down && exit 1)
	@docker-compose -f docker-compose.test.yml down
	@echo "Tests completed"
