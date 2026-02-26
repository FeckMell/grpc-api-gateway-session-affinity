# Stage 1: Builder
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS builder

WORKDIR /src

# Copy solution file
COPY MyService.sln .

# Copy project file
COPY MyService/MyService.csproj MyService/

# Restore dependencies
RUN dotnet restore MyService/MyService.csproj

# Copy source code
COPY MyService/ MyService/

# Build and publish the application
RUN dotnet publish MyService/MyService.csproj -c Release -o /app/publish --no-restore

# Stage 2: Runtime
FROM mcr.microsoft.com/dotnet/aspnet:8.0

# Create non-root user
RUN groupadd -r -g 1000 nonroot && \
    useradd -r -u 1000 -g nonroot nonroot

WORKDIR /app

# Copy published application from builder stage
COPY --from=builder /app/publish .

# Change ownership to nonroot user
RUN chown -R nonroot:nonroot /app

# Switch to non-root user
USER nonroot:nonroot

# Expose port
EXPOSE 5000

# Run the application
ENTRYPOINT ["dotnet", "MyService.dll"]
