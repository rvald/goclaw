# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy gomod and sum
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically linked binary
# CGO_ENABLED=0 for static binary
# -ldflags="-w -s" to strip debug info and reduce size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o goclaw ./cmd/goclaw

# Final stage
FROM gcr.io/distroless/static-debian12

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/goclaw /app/goclaw

# Expose port (default 18789)
EXPOSE 18789

# Run as non-root (distroless provides 'nonroot' user: 65532)
USER nonroot:nonroot

# Volume for state
VOLUME ["/data"]

# Set state dir to volume
ENV XDG_STATE_HOME=/data

ENTRYPOINT ["/app/goclaw"]
