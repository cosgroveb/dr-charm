.PHONY: build install fmt test test-release agent-sloc app-sloc clean

VERSION ?= dev
AGENT_SLOC_LIMIT := 250
APP_SLOC_LIMIT := 5900

build:
	go build -ldflags "-X main.version=$(VERSION)" -o dr-charm ./cmd/dr-charm

fmt:
	go fmt ./...

test:
	go test -race -shuffle=on ./...

test-release:
	scripts/test-release

agent-sloc:
	@count=$$(find internal/agent -name '*.go' ! -name '*_test.go' -print0 | xargs -0 cloc --quiet --timeout 30 --csv --include-lang=Go | awk -F, '$$2 == "Go" {print $$5}'); \
	echo "agent SLOC: $$count / $(AGENT_SLOC_LIMIT)"; \
	test "$$count" -le "$(AGENT_SLOC_LIMIT)"

app-sloc:
	@count=$$(find cmd internal -name '*.go' ! -name '*_test.go' -print0 | xargs -0 cloc --quiet --timeout 30 --csv --include-lang=Go | awk -F, '$$2 == "Go" {print $$5}'); \
	echo "app SLOC: $$count / $(APP_SLOC_LIMIT)"; \
	test "$$count" -le "$(APP_SLOC_LIMIT)"

clean:
	rm -f dr-charm

install:
	go install ./cmd/dr-charm
