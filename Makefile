.PHONY: build test lint clean install

VERSION ?= dev

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/claim ./cmd/claim/

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf bin/ dist/

install: build
	cp bin/claim /usr/local/bin/claim

snapshot:
	goreleaser build --snapshot --clean
