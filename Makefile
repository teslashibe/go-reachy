# go-reachy Makefile
# Common development and deployment commands

.PHONY: all build test lint fmt clean deploy run help

# Default target
all: lint test build

# Build eva binary
build:
	go build -o bin/eva ./cmd/eva

# Build for ARM64 (Reachy Mini robot)
build-arm:
	GOOS=linux GOARCH=arm64 go build -o bin/eva-arm64 ./cmd/eva

# Build all commands
build-all:
	@for cmd in $$(ls cmd); do \
		echo "Building $$cmd..."; \
		go build -o bin/$$cmd ./cmd/$$cmd; \
	done

# Run tests
test:
	go test -race ./...

# Run tests with coverage
test-coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run integration tests (requires API keys)
test-integration:
	go test -race -tags=integration ./...

# Lint code
lint:
	go vet ./...
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Code is not formatted. Run 'make fmt'"; \
		gofmt -l .; \
		exit 1; \
	fi

# Format code
fmt:
	gofmt -w .

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Deploy to robot
deploy: build-arm
	@if [ -z "$(ROBOT_IP)" ]; then \
		echo "Error: ROBOT_IP not set"; \
		echo "Usage: ROBOT_IP=192.168.68.80 make deploy"; \
		exit 1; \
	fi
	scp bin/eva-arm64 pollen@$(ROBOT_IP):~/eva
	@echo "Deployed to $(ROBOT_IP)"

# Run eva locally
run:
	go run ./cmd/eva

# Run dance demo
run-dance:
	go run ./cmd/dance

# Run test-elevenlabs
run-test-elevenlabs:
	go run ./cmd/test-elevenlabs

# Update dependencies
deps-update:
	go get -u ./...
	go mod tidy

# Check for security vulnerabilities
security:
	@command -v govulncheck >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# Generate documentation
docs:
	@echo "Opening godoc server at http://localhost:6060"
	godoc -http=:6060

# Help
help:
	@echo "go-reachy Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make              Build, lint, and test"
	@echo "  make build        Build eva binary"
	@echo "  make build-arm    Build for ARM64 (robot)"
	@echo "  make test         Run tests"
	@echo "  make test-coverage Run tests with coverage report"
	@echo "  make lint         Run linter"
	@echo "  make fmt          Format code"
	@echo "  make clean        Remove build artifacts"
	@echo "  make deploy       Deploy to robot (ROBOT_IP required)"
	@echo "  make run          Run eva locally"
	@echo "  make deps-update  Update dependencies"
	@echo "  make security     Check for vulnerabilities"
	@echo "  make help         Show this help"




