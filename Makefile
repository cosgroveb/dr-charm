.PHONY: all build clean fmt test dr-charm

# Default target
all: build

# Build the dr-charm binary
build: dr-charm

# Build the main dr-charm client
dr-charm:
	go build -o dr-charm .


# Format code
fmt:
	go fmt ./...

# Run tests via dagger
test:
	dagger call test --source .

# Clean build artifacts
clean:
	rm -f dr-charm

# Install dr-charm
install: build
	go install .

# Run with debug mode
debug:
	DR_CHARM_DEBUG=true ./dr-charm

# Run performance test
perf-test:
	DR_CHARM_PERF_TEST=true go run .

# Run in CLI mode
cli-mode:
	DR_CHARM_CLI=true ./dr-charm