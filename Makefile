.PHONY: all build test clean lint deps

# Binary name
BINARY_NAME=jsonql-gen-sql

all: test build

build:
	@echo "Building..."
	go build -o bin/$(BINARY_NAME) ./cmd/jsonql-gen-sql

test:
	@echo "Running tests..."
	go test -v ./...

clean:
	@echo "Cleaning..."
	go clean
	rm -rf bin/

lint:
	@echo "Linting..."
	go vet ./...

format:
	@echo "Formatting..."
	go fmt ./...

deps:
	@echo "Downloading dependencies..."
	go mod download
