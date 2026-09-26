# Multi-stage Dockerfile for AI-Ops Copilot Agent Backend
# Stage 1: Build static binary
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install certificates and git
RUN apk add --no-cache ca-certificates git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary without CGO
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /app/bin/server ./cmd/server

# Stage 2: Distroless minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy compiled binary from builder
COPY --from=builder /app/bin/server /app/server
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/app/server"]
