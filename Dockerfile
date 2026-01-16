# syntax=docker/dockerfile:1

# renovate: datasource=docker depName=golang
ARG GOLANG_VERSION=1.23

# renovate: datasource=github-releases depName=restic/restic
ARG RESTIC_VERSION=0.17.3

# Build stage
FROM golang:${GOLANG_VERSION}-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o gorestic-homelab ./cmd/gorestic-homelab

# Final stage
FROM alpine:3.20

# renovate: datasource=github-releases depName=restic/restic
ARG RESTIC_VERSION=0.17.3

LABEL org.opencontainers.image.source=https://github.com/fgeck/gorestic-homelab
LABEL org.opencontainers.image.description="A restic backup orchestrator for homelab environments"
LABEL org.opencontainers.image.licenses=MIT

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    restic \
    postgresql16-client \
    tzdata

# Create non-root user
RUN adduser -D -u 1000 gorestic

# Copy binary from builder
COPY --from=builder /app/gorestic-homelab /usr/local/bin/gorestic-homelab

# Set ownership
RUN chown gorestic:gorestic /usr/local/bin/gorestic-homelab

# Switch to non-root user
USER gorestic

ENTRYPOINT ["gorestic-homelab"]
CMD ["--help"]
