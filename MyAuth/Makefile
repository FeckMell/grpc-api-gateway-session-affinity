# MyAuth build and code generation.
# Before first `make generate`: run `make install`, add GOPATH/bin to PATH, install protoc (brew/chocolatey).
# For generate, protoc must be on PATH; protoc-gen-go and protoc-gen-go-grpc are installed by make install.

.PHONY: install check-go install-tools generate build help test

# Version variables - update these to bump versions
GO_VERSION := 1.22
MOQ_VERSION := v0.5.3

# Show help message
help:
	@echo "MyAuth Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  make help     - Show this help message"
	@echo "  make generate  - Generate Go code from api.proto and mocks"
	@echo "  make test     - Run all tests"
	@echo "  make build    - Build myauth binary"
	@echo ""
	@echo "First time setup:"
	@echo "  1. Install Go $(GO_VERSION) or later: https://go.dev/dl/"
	@echo "  2. Run 'make install' to install development tools"
	@echo "  3. Add Go binaries to PATH (see instructions after 'make install')"
	@echo "  4. Install protoc if needed (macOS: brew install protobuf; Windows: chocolatey install protoc)"
	@echo "  5. Run 'make generate' to generate code"

# Generate code from proto (via //go:generate in handlers/grpc.go) and mocks
generate:
	@echo "+++ MyAuth: Generating code"
	@go generate ./...
	@echo "+++ MyAuth: Code generation complete"

# Run all tests
test:
	@echo "+++ MyAuth: Running tests..."
	@go test -v ./...
	@echo "+++ MyAuth: Tests completed"

# Build the binary
build:
	@echo "+++ MyAuth: Building..."
	@go build -ldflags="-s -w" -o myauth ./cmd
	@echo "+++ MyAuth: Build complete"