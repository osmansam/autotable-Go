# ---------- build stage ----------
FROM golang:1.22 AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
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
