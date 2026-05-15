.PHONY: all build test e2e clean lint fmt vet coverage install tidy deps demo-deps demo-clean

# Variables
BINARY_NAME := airgapctl
MAIN_PACKAGE := ./cmd/airgapctl
BUILD_DIR := ./bin
GO := go
GOFLAGS :=
LDFLAGS :=

# Default target
all: build

# Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

# Run tests (excludes E2E suite)
test:
	$(GO) test -v -race $(shell $(GO) list ./... | grep -v "tests/e2e")

# Run E2E tests
e2e:
	$(GO) test -v ./tests/e2e/...

# Run tests with coverage
coverage:
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# Format code
fmt:
	$(GO) fmt ./...

# Run go vet
vet:
	$(GO) vet ./...

# Run linter (requires golangci-lint)
lint: fmt vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

# Install binary to GOPATH/bin
install:
	$(GO) install $(GOFLAGS) -ldflags "$(LDFLAGS)" $(MAIN_PACKAGE)

# Tidy go modules
tidy:
	$(GO) mod tidy

# Download dependencies
deps:
	$(GO) mod download

# Run all checks (format, vet, test)
check: fmt vet test

# Check for VHS demo dependencies and print install instructions when missing
demo-deps:
	@echo "Checking VHS demo dependencies..."
	@if command -v vhs >/dev/null 2>&1; then \
		echo "  ✓ vhs is installed"; \
	else \
		echo "  ✗ vhs is missing. Install with: brew install charmbracelet/tap/vhs"; \
	fi
	@if command -v ttyd >/dev/null 2>&1; then \
		echo "  ✓ ttyd is installed"; \
	else \
		echo "  ✗ ttyd is missing. Install with: brew install ttyd"; \
	fi

# Generate demo fixtures (mock bundle and Helm chart)
demo-fixtures:
	@echo "Generating demo fixtures..."
	$(GO) run demo/generate-bundle.go

# Clean generated demo artifacts
demo-clean:
	@echo "Cleaning demo artifacts..."
	@rm -f demo/*.gif
	@rm -f demo/fixtures/*.airgap
	@rm -f demo/airgapctl
	@rm -f demo/generate-bundle
	@echo "Done."
