.PHONY: build run test test-coverage docker-build docker-run clean deps fmt lint vet security qa dev help

help:
	@echo "Available targets:"
	@echo "  make build              - Build the application"
	@echo "  make run                - Run the application locally"
	@echo "  make dev                - Run with hot reload (requires air)"
	@echo "  make test               - Run unit tests"
	@echo "  make test-coverage      - Run tests with coverage report"
	@echo "  make vet                - Run go vet analysis"
	@echo "  make fmt                - Format code"
	@echo "  make fmt-check          - Check code formatting without modifying"
	@echo "  make lint               - Run golangci-lint"
	@echo "  make security           - Run security checks (gosec)"
	@echo "  make qa                 - Run all QA checks (test, fmt, vet, lint)"
	@echo "  make deps               - Download dependencies and install QA/dev tools"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make docker-build       - Build Docker image"
	@echo "  make docker-run         - Run Docker container"

# Build the application
build:
	go build -v -o cargo-server ./cmd/server

# Run the application locally
run:
	go run ./cmd/server

# Run with hot reload (requires air - install with: go install github.com/air-verse/air@latest)
dev:
	~/go/bin/air

# Run unit tests
test:
	go test -v -race ./...

# Run tests with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run go vet
vet:
	go vet ./...

# Install dependencies
deps:
	go mod tidy
	go mod download
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/air-verse/air@latest

# Format code
fmt:
	go fmt ./...

# Check code formatting without modifying
fmt-check:
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "Go code is not formatted:"; \
		gofmt -s -d .; \
		exit 1; \
	fi

# Lint code (requires golangci-lint - install with: make install)
lint:
	~/go/bin/golangci-lint run ./...

# Security checks (requires gosec - install with: make install)
security:
	~/go/bin/gosec -no-fail ./...

# Run all QA checks
qa: fmt-check vet test lint
	@echo "✅ All QA checks passed!"

# Build Docker image
docker-build:
	docker build -t cargo-mleczki:latest .

# Run Docker container
docker-run:
	docker run -p 8080:8080 \
		-v $(PWD)/data:/app/data \
		-v $(PWD)/db:/app/db \
		cargo-mleczki:latest

# Clean build artifacts
clean:
	rm -f cargo-server
	rm -f *.db
	rm -f *.db-shm
	rm -f *.db-wal
	rm -rf tmp/
	rm -f coverage.out
	rm -f coverage.html

lint-fix:
	make fmt
	~/go/bin/golangci-lint run --fix ./...
