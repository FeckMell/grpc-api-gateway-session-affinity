# Stage 1: Builder
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mygateway ./cmd

# Stage 2: Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates

RUN addgroup -g 1000 nonroot && \
    adduser -D -u 1000 -G nonroot nonroot

WORKDIR /app

COPY --from=builder /build/mygateway .

RUN chown nonroot:nonroot mygateway

USER nonroot:nonroot

EXPOSE 10000

CMD ["./mygateway"]
