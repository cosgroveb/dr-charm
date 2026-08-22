.PHONY: build install fmt test clean

build:
	go build -o dr-charm ./cmd/dr-charm

fmt:
	go fmt ./...

test:
	go test -race -shuffle=on ./...

clean:
	rm -f dr-charm

install:
	go install ./cmd/dr-charm
