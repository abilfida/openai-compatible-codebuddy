# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . ./

# Build the binary
RUN go build -o server ./cmd/server

# Runtime stage
FROM alpine:3.19

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy binary
COPY --from=builder /app/server /server

# Expose port
EXPOSE 3000

# Set default environment
ENV PORT=3000
ENV HOST=0.0.0.0

# Run the server
CMD ["/server"]