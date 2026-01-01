# Dockerfile - Simple multi-stage build for Eva
#
# Build:  docker compose build
# Run:    docker compose up
#
# Or standalone:
#   docker build -t eva .
#   docker run --network host -e OPENAI_API_KEY=xxx eva

# ============================================
# Stage 1: Build
# ============================================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /eva ./cmd/eva

# ============================================
# Stage 2: Runtime
# ============================================
FROM alpine:3.19

# Install runtime dependencies for audio playback
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sshpass \
    openssh-client \
    gstreamer \
    gst-plugins-base \
    gst-plugins-good \
    gst-plugins-bad \
    gst-plugins-ugly \
    ffmpeg

# Copy binary from builder
COPY --from=builder /eva /usr/local/bin/eva

# Expose web dashboard port
EXPOSE 8181

# Default command
CMD ["/usr/local/bin/eva"]


