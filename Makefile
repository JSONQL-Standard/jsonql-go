.PHONY: all build test clean lint deps

# Binary name
BINARY_NAME=jsonql-gen-sql

all: test build

build:
	@echo "Building..."
	go build -o bin/$(BINARY_NAME) ./cmd/jsonql-gen-sql

test:
	@if command -v gotestsum > /dev/null; then \
		gotestsum --format testname -- ./...; \
	elif [ -f $(HOME)/go/bin/gotestsum ]; then \
		$(HOME)/go/bin/gotestsum --format testname -- ./...; \
	else \
		echo "Running tests (install gotestsum for nicer output)..."; \
		go test -v ./...; \
	fi

test-watch:
	@if command -v gotestsum > /dev/null; then \
		gotestsum --watch -- ./...; \
	elif [ -f $(HOME)/go/bin/gotestsum ]; then \
		$(HOME)/go/bin/gotestsum --watch -- ./...; \
	else \
		echo "gotestsum not found. Please install it: go install gotest.tools/gotestsum@latest"; \
		exit 1; \
	fi

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
