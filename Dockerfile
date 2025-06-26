# Use a specific Go version for consistency, e.g., golang:1.22-alpine
# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application, specifying the main package path
# The output binary will be named 'email-service' (or 'main' if you prefer)
RUN go build -o email-service ./cmd/main.go

# Run stage
FROM alpine:latest

WORKDIR /app

# Copy only the built binary from the builder stage
COPY --from=builder /app/email-service .

# Expose the port the application listens on
EXPOSE 7979

# Command to run the application
CMD ["./email-service"]