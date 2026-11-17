# syntax=docker/dockerfile:1

# ---------- build stage ----------
FROM golang:1.24 AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN --mount=type=cache,id=autotable-go-mod,target=/go/pkg/mod,sharing=locked \
    go mod download

# Copy the entire source code
COPY . .

# Build the application
# Reuse caches without writing them into a new intermediate image layer per build.
RUN --mount=type=cache,id=autotable-go-mod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=autotable-go-build,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o server .

# ---------- runtime stage ----------
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/server .

# Copy configuration files
COPY --from=builder /app/configs ./configs

# Create temp directory for file uploads
RUN mkdir -p /app/temp /app/logs && \
    chmod 755 /app/temp /app/logs

# Expose the port your app runs on
EXPOSE 4000

# Run the application
ENTRYPOINT ["/app/server"]
