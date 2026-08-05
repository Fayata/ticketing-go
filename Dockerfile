# Stage 1: Builder
FROM golang:1.25-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build all service binaries
RUN go build -o /bin/gateway ./cmd/gateway && \
    go build -o /bin/auth-service ./cmd/auth-service && \
    go build -o /bin/ticket-service ./cmd/ticket-service && \
    go build -o /bin/notification-service ./cmd/notification-service && \
    go build -o /bin/admin-service ./cmd/admin-service

# Stage 2: Runtime
FROM alpine:3.20

# Install tzdata for timezone support
RUN apk add --no-cache tzdata

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /bin/gateway /bin/gateway
COPY --from=builder /bin/auth-service /bin/auth-service
COPY --from=builder /bin/ticket-service /bin/ticket-service
COPY --from=builder /bin/notification-service /bin/notification-service
COPY --from=builder /bin/admin-service /bin/admin-service

# Copy templates and static directories
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static

# Define ARG to select which binary to run at startup
ARG SERVICE_NAME=gateway
ENV SERVICE_NAME=${SERVICE_NAME}

# EXPOSE default port, this is overridden by docker-compose for each service
EXPOSE 8080

# Command to run the selected service
CMD sh -c "/bin/${SERVICE_NAME}"