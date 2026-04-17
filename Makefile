VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X github.com/Abraxas-365/claudio/internal/cli.Version=$(VERSION)"

.PHONY: build run test clean install

build:
	go build $(LDFLAGS) -o bin/claudio ./cmd/claudio

run:
	go run $(LDFLAGS) ./cmd/claudio

install:
	go install $(LDFLAGS) ./cmd/claudio

test:
	go test ./...

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

# Cross-compilation
build-all:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/claudio-darwin-arm64 ./cmd/claudio
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/claudio-darwin-amd64 ./cmd/claudio
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/claudio-linux-amd64 ./cmd/claudio
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/claudio-linux-arm64 ./cmd/claudio
