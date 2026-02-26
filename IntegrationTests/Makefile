# IntegrationTests: scenario-based integration tests for MyService.
# Requires: Go 1.22+, protoc, protoc-gen-go, protoc-gen-go-grpc (make install from root).

.PHONY: generate build test help

help:
	@echo "IntegrationTests Makefile"
	@echo ""
	@echo "  make generate - Generate Go code from api/Api.proto"
	@echo "  make build   - Build integrationtests binary"
	@echo "  make test    - Run tests"

generate:
	@echo "+++ IntegrationTests: Generating code from proto"
	@protoc --proto_path=. api/Api.proto \
		--go_out=. --go_opt=module=integrationtests \
		--go-grpc_out=. --go-grpc_opt=module=integrationtests
	@echo "+++ IntegrationTests: Code generation complete"

build:
	@echo "+++ IntegrationTests: Building..."
	@go build -ldflags="-s -w" -o integrationtests ./cmd
	@echo "+++ IntegrationTests: Build complete"

test:
	@echo "+++ IntegrationTests: Running tests..."
	@go test -v ./...
	@echo "+++ IntegrationTests: Tests completed"
