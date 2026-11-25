# syntax=docker/dockerfile:1

# ---------- build stage ----------
FROM golang:1.26.11-alpine3.24@sha256:ecb67f987aa1a57794c91d19ade7ae0209f2f0f5496aaef9e6ae85ddf92d02ab AS builder

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
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# Apply stable-branch security fixes published after the pinned image snapshot.
# Installing certificates alone does not upgrade vulnerable packages in the base.
RUN apk --no-cache upgrade && \
    apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/server .

# Copy configuration files
COPY --from=builder /app/configs/*.json ./configs/

# Create temp directory for file uploads
RUN addgroup -S -g 10001 autotable && \
    adduser -S -D -H -u 10001 -G autotable autotable && \
    mkdir -p /app/temp /app/logs && \
    chown 10001:10001 /app/temp /app/logs && \
    chmod 755 /app/temp /app/logs

USER 10001:10001

# Expose the port your app runs on
EXPOSE 4000

# Run the application
ENTRYPOINT ["/app/server"]
