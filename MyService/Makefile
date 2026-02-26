# MyService build and development commands.
# Requires .NET 8 SDK: https://dotnet.microsoft.com/download

.PHONY: help build test run clean

# Show help message
help:
	@echo "MyService Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  make help     - Show this help message"
	@echo "  make build    - Build MyService project"
	@echo "  make test     - Run all tests"
	@echo "  make run      - Run MyService locally"
	@echo "  make clean    - Clean build artifacts"
	@echo ""
	@echo "Requirements:"
	@echo "  .NET 8 SDK: https://dotnet.microsoft.com/download"

# Build the project
build:
	@echo "+++ MyService: Building..."
	@dotnet build MyService/MyService.csproj -c Release
	@echo "+++ MyService: Build complete"

# Run all tests
test:
	@echo "+++ MyService: Running tests..."
	@dotnet test
	@echo "+++ MyService: Tests completed"

# Run the application locally
run:
	@echo "+++ MyService: Running locally..."
	@dotnet run --project MyService/MyService.csproj

# Clean build artifacts
clean:
	@echo "+++ MyService: Cleaning..."
	@dotnet clean
	@echo "+++ MyService: Clean complete"
