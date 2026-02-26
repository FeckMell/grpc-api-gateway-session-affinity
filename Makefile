# Root Makefile for the whole solution.
# install: all tools for Go projects. generate/test/build: delegate to project Makefiles.
# up: requires docker-compose.yml in the root to bring up the solution.

.PHONY: install check-go install-tools test test_auth test_discoverer test_mygateway test_myservice test_integrationtests generate generate_auth generate_discoverer generate_mygateway generate_integrationtests build_myservice build_integrationtests build_mygateway run_myservice clean_myservice up down rebuild itest itest_basic_workflow itest_sticky_session itest_system_overload itest_stream_subscription itest_stream_subscription_myservice_stop itest_login_errors itest_myservice_auth_errors itest_session_transfer itest_gateway_error_unauthenticated itest_gateway_error_unavailable itest_myservice_shutdown_errors itest_no_instances_available help

# Version variables (aligned with MyDiscoverer and MyAuth)
GO_VERSION := 1.25
OAPI_CODEGEN_VERSION := v1.16.2
MOQ_VERSION := v0.5.3
PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.6.1

.DEFAULT_GOAL := help

help:
	@echo "Solution Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  make help                - Show this help message"
	@echo "  make install             - Install all required tools (Go $(GO_VERSION)+, oapi-codegen, moq, protoc-gen-go, protoc-gen-go-grpc)"
	@echo "  make generate            - Run code generation for all projects"
	@echo "  make generate_auth       - Run MyAuth code generation only"
	@echo "  make generate_discoverer - Run MyDiscoverer code generation only"
	@echo "  make generate_mygateway  - Run MyGateway code generation (mocks) only"
	@echo "  make generate_integrationtests - Run IntegrationTests code generation only"
	@echo "  make rebuild             - Rebuild all images (MyAuth, MyDiscoverer, MyGateway, MyService) for integration tests"
	@echo "  make test                - Run all tests"
	@echo "  make test_auth           - Run MyAuth tests only"
	@echo "  make test_discoverer     - Run MyDiscoverer tests only"
	@echo "  make test_mygateway      - Run MyGateway tests only"
	@echo "  make test_myservice      - Run MyService tests only"
	@echo "  make test_integrationtests  - Run IntegrationTests tests only"
	@echo "  make run_myservice      - Run MyService locally"
	@echo "  make clean_myservice    - Clean MyService build artifacts"
	@echo "  make up                 - Run docker-compose up (requires docker-compose.yml in root)"
	@echo "  make down               - Shut down solution (docker-compose down)"
	@echo "  make itest                     - Run all integration test scenarios"
	@echo "  make itest_basic_workflow      - Run basic_workflow integration test scenario"
	@echo "  make itest_sticky_session      - Run sticky_session integration test scenario"
	@echo "  make itest_system_overload     - Run system_overload integration test scenario"
	@echo "  make itest_stream_subscription - Run stream_subscription integration test scenario"
	@echo "  make itest_stream_subscription_myservice_stop - Run stream_subscription_myservice_stop scenario (requires compose)"
	@echo "  make itest_login_errors          - Run login_errors integration test scenario"
	@echo "  make itest_myservice_auth_errors - Run myservice_authentication_errors integration test scenario"
	@echo "  make itest_session_transfer      - Run session_transfer integration test scenario"
	@echo "  make itest_gateway_error_unauthenticated - Run gateway_error_unauthenticated scenario"
	@echo "  make itest_gateway_error_unavailable     - Run gateway_error_unavailable scenario (requires compose)"
	@echo "  make itest_myservice_shutdown_errors     - Run myservice_shutdown_errors scenario"
	@echo "  make itest_no_instances_available        - Run no_instances_available scenario"
	@echo ""
	@echo "First time: make install, add GOPATH/bin to PATH, install protoc if using MyAuth generate. For MyService: .NET 8 SDK (https://dotnet.microsoft.com/download)."

# Install all required tools for the whole solution
install: check-go install-tools
	@echo ""
	@echo "All tools installed successfully!"
	@echo ""

check-go:
	@echo "Checking Go installation..."
	@which go >/dev/null 2>&1 || command -v go >/dev/null 2>&1 || { \
		echo "ERROR: Go is not installed."; \
		echo ""; \
		echo "Please install Go $(GO_VERSION) or later:"; \
		echo "  macOS:   brew install go@$(GO_VERSION) or download from https://go.dev/dl/"; \
		echo "  Windows: Download from https://go.dev/dl/"; \
		echo "  Linux:   See https://go.dev/doc/install"; \
		exit 1; \
	}
	@echo "Go is installed: $$(go version)"
	@GO_CURRENT=$$(go version | awk '{print $$3}' | sed 's/go//' 2>/dev/null || go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//' 2>/dev/null || echo ""); \
	if [ -n "$$GO_CURRENT" ]; then \
		GO_REQUIRED=$(GO_VERSION); \
		GO_MAJOR_CURRENT=$$(echo $$GO_CURRENT | cut -d. -f1); \
		GO_MINOR_CURRENT=$$(echo $$GO_CURRENT | cut -d. -f2); \
		GO_MAJOR_REQUIRED=$$(echo $$GO_REQUIRED | cut -d. -f1); \
		GO_MINOR_REQUIRED=$$(echo $$GO_REQUIRED | cut -d. -f2); \
		if [ $$GO_MAJOR_CURRENT -lt $$GO_MAJOR_REQUIRED ] || \
		   ([ $$GO_MAJOR_CURRENT -eq $$GO_MAJOR_REQUIRED ] && [ $$GO_MINOR_CURRENT -lt $$GO_MINOR_REQUIRED ]); then \
			echo "WARNING: Go version $$GO_CURRENT detected, but $(GO_VERSION) or later is recommended."; \
			echo "Please upgrade Go: https://go.dev/dl/"; \
			echo ""; \
			echo "Continuing anyway..."; \
		fi; \
	fi

install-tools:
	@echo "Installing development tools for the solution..."
	@echo ""
	@echo "Installing oapi-codegen (MyDiscoverer)..."
	@go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) || { \
		echo "ERROR: Failed to install oapi-codegen"; \
		exit 1; \
	}
	@echo "Installing moq..."
	@go install github.com/matryer/moq@$(MOQ_VERSION) || { \
		echo "ERROR: Failed to install moq"; \
		exit 1; \
	}
	@echo "Installing protoc-gen-go (MyAuth)..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION) || { \
		echo "ERROR: Failed to install protoc-gen-go"; \
		exit 1; \
	}
	@echo "Installing protoc-gen-go-grpc (MyAuth)..."
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION) || { \
		echo "ERROR: Failed to install protoc-gen-go-grpc"; \
		exit 1; \
	}
	@echo ""
	@echo "Tools installed successfully!"
	@echo ""
	@GOPATH_BIN=$$(go env GOPATH)/bin; \
	GOBIN=$$(go env GOBIN); \
	if [ -n "$$GOBIN" ] && [ "$$GOBIN" != "" ] && [ "$$GOBIN" != "null" ]; then \
		BIN_PATH=$$GOBIN; \
	else \
		BIN_PATH=$$GOPATH_BIN; \
	fi; \
	echo "Tools installed to: $$BIN_PATH"
	@echo ""
	@echo "To add Go binaries to your PATH:"
	@echo "  macOS/Linux:  export PATH=$$PATH:$$(go env GOPATH)/bin"
	@echo "  Windows:     add $$(go env GOPATH)\\bin to PATH"
	@echo ""
	@echo "For MyAuth generate you also need protoc on PATH (e.g. macOS: brew install protobuf; Windows: chocolatey install protoc)."

# Code generation
generate: generate_discoverer generate_auth generate_mygateway generate_integrationtests

generate_auth:
	@$(MAKE) -C MyAuth generate

generate_discoverer:
	@$(MAKE) -C MyDiscoverer generate

generate_mygateway:
	@$(MAKE) -C MyGateway generate

generate_integrationtests:
	@$(MAKE) -C IntegrationTests generate

# Tests
test: test_discoverer test_auth test_mygateway test_integrationtests #test_myservice

test_auth:
	@$(MAKE) -C MyAuth test

test_discoverer:
	@$(MAKE) -C MyDiscoverer test

test_mygateway:
	@$(MAKE) -C MyGateway test

test_integrationtests:
	@$(MAKE) -C IntegrationTests test

test_myservice:
	@$(MAKE) -C MyService test

# Rebuild all service images so integration tests run with the latest code
rebuild:
	docker-compose build myauth mydiscoverer mygateway myservice

run_myservice:
	@$(MAKE) -C MyService run

clean_myservice:
	@$(MAKE) -C MyService clean

# Build integration test binary (required for itest_* targets)
build_integrationtests:
	@$(MAKE) -C IntegrationTests build

# Bring up the whole solution (requires docker-compose.yml in root)
up:
	docker-compose up

# Shut down the solution (for restart: make down && make up)
down:
	docker-compose down

# Integration tests
itest: build_integrationtests
	@echo "Running all integration test scenarios..."
	@cd IntegrationTests && \
		scenarios=$$(./integrationtests --list); \
		failed=0; \
		for scenario in $$scenarios; do \
			echo ""; \
			echo "=== Running scenario: $$scenario ==="; \
			if ! ./integrationtests --compose-file=../docker-compose.yml $$scenario; then \
				echo "Scenario $$scenario failed"; \
				failed=$$((failed + 1)); \
			fi; \
		done; \
		if [ $$failed -eq 0 ]; then \
			echo ""; \
			echo "All scenarios passed!"; \
		else \
			echo ""; \
			echo "$$failed scenario(s) failed"; \
			exit 1; \
		fi

itest_basic_workflow: build_integrationtests
	@echo "Running basic_workflow integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml basic_workflow

itest_sticky_session: build_integrationtests
	@echo "Running sticky_session integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml sticky_session

itest_system_overload: build_integrationtests
	@echo "Running system_overload integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml system_overload

itest_stream_subscription: build_integrationtests
	@echo "Running stream_subscription integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml stream_subscription

itest_stream_subscription_myservice_stop: build_integrationtests
	@echo "Running stream_subscription_myservice_stop integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml stream_subscription_myservice_stop

itest_login_errors: build_integrationtests
	@echo "Running login_errors integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml login_errors

itest_myservice_auth_errors: build_integrationtests
	@echo "Running myservice_authentication_errors integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml myservice_authentication_errors

itest_session_transfer: build_integrationtests
	@echo "Running session_transfer integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml session_transfer

itest_gateway_error_unauthenticated: build_integrationtests
	@echo "Running gateway_error_unauthenticated integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml gateway_error_unauthenticated

itest_gateway_error_unavailable: build_integrationtests
	@echo "Running gateway_error_unavailable integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml gateway_error_unavailable

itest_myservice_shutdown_errors: build_integrationtests
	@echo "Running myservice_shutdown_errors integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml myservice_shutdown_errors

itest_no_instances_available: build_integrationtests
	@echo "Running no_instances_available integration test scenario..."
	@cd IntegrationTests && ./integrationtests --compose-file=../docker-compose.yml no_instances_available
