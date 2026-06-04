.PHONY: build run test docker-build docker-run clean deps

# Build the application
build:
	go build -o cargo-server ./cmd/server

# Run the application locally
run:
	go run ./cmd/server

# Run tests
test:
	go test -v ./...

# Install dependencies
deps:
	go mod tidy
	go mod download

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
	rm -rf tmp/

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run ./...

# Run with hot reload (requires air)
dev:
	air
