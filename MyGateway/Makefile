# MyGateway build and code generation.

.PHONY: generate test build help

help:
	@echo "MyGateway Makefile"
	@echo ""
	@echo "  make generate  - Generate mocks (go generate ./...)"
	@echo "  make test      - Run all tests"
	@echo "  make build     - Build mygateway binary"
	@echo ""

generate:
	@echo "+++ MyGateway: Generating code..."
	@go generate ./...
	@echo "+++ MyGateway: Code generation complete"

test:
	@echo "+++ MyGateway: Running tests..."
	@go test -v ./...
	@echo "+++ MyGateway: Tests completed"

build:
	@echo "+++ MyGateway: Building..."
	@go build -ldflags="-s -w" -o mygateway ./cmd
	@echo "+++ MyGateway: Build complete"
