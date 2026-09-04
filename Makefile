.PHONY: build install fmt test test-release clean

VERSION ?= dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o dr-charm ./cmd/dr-charm

fmt:
	go fmt ./...

test:
	go test -race -shuffle=on ./...

test-release:
	scripts/test-release

clean:
	rm -f dr-charm

install:
	go install ./cmd/dr-charm
