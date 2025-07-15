.PHONY: all build clean fmt test drcli dr-charm

# Default target
all: build

# Build all binaries
build: dr-charm drcli

# Build the main dr-charm client
dr-charm:
	go build -o dr-charm .

# Build the CLI tool
drcli:
	go build -o drcli ./cmd/drcli

# Format code
fmt:
	go fmt ./...

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -f dr-charm drcli

# Install both tools
install: build
	go install .
	go install ./cmd/drcli

# Run with debug mode
debug:
	DR_CHARM_DEBUG=true ./dr-charm

# Run performance test
perf-test:
	DR_CHARM_PERF_TEST=true go run .

# Run in CLI mode
cli-mode:
	DR_CHARM_CLI=true ./dr-charm