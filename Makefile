# Variables
GO=go
GOFLAGS=-v

.PHONY: all build-all build-gateway build-auth build-ticket build-notification build-admin \
	run-gateway run-auth run-ticket run-notification run-admin run-all run-monolith \
	docker-up docker-down test lint clean

# Default target
all: build-all

# Build Targets
build-all: build-gateway build-auth build-ticket build-notification build-admin

build-gateway:
	$(GO) build $(GOFLAGS) -o bin/gateway ./cmd/gateway

build-auth:
	$(GO) build $(GOFLAGS) -o bin/auth-service ./cmd/auth-service

build-ticket:
	$(GO) build $(GOFLAGS) -o bin/ticket-service ./cmd/ticket-service

build-notification:
	$(GO) build $(GOFLAGS) -o bin/notification-service ./cmd/notification-service

build-admin:
	$(GO) build $(GOFLAGS) -o bin/admin-service ./cmd/admin-service

# Run Targets
run-gateway:
	./bin/gateway

run-auth:
	./bin/auth-service

run-ticket:
	./bin/ticket-service

run-notification:
	./bin/notification-service

run-admin:
	./bin/admin-service

run-all: build-all
	./bin/gateway & \
	./bin/auth-service & \
	./bin/ticket-service & \
	./bin/notification-service & \
	./bin/admin-service & \
	wait

run-monolith:
	$(GO) run main.go

# Docker Targets
docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down

# Dev Tools
test:
	$(GO) test ./... $(GOFLAGS)

lint:
	$(GO) vet ./...

clean:
	rm -rf bin/
