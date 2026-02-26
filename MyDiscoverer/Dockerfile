# Stage 1: Builder
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mydiscoverer ./cmd

# Stage 2: Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates

RUN addgroup -g 1000 nonroot && \
    adduser -D -u 1000 -G nonroot nonroot

WORKDIR /app

COPY --from=builder /build/mydiscoverer .
RUN chown nonroot:nonroot mydiscoverer

USER nonroot:nonroot

EXPOSE 8080

CMD ["./mydiscoverer"]
