# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /eva-cloud ./cmd/eva-cloud

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /eva-cloud /app/eva-cloud

# Copy static files if any
COPY --from=builder /app/web/static /app/web/static 2>/dev/null || true

# Create non-root user
RUN adduser -D -H eva
USER eva

# Expose ports
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

# Environment variables
ENV PORT=8080
ENV LOG_LEVEL=info

# Run
ENTRYPOINT ["/app/eva-cloud"]


