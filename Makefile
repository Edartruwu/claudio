VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X github.com/Abraxas-365/claudio/internal/cli.Version=$(VERSION)"

.PHONY: build run test clean install reload

build:
	templ generate ./internal/comandcenter/web/...
	go build $(LDFLAGS) -o bin/claudio ./cmd/claudio
	go build $(LDFLAGS) -o bin/claudio-server ./cmd/comandcenter

templ-install:
	go install github.com/a-h/templ/cmd/templ@latest

run:
	go run $(LDFLAGS) ./cmd/claudio

run-server:
	go run $(LDFLAGS) ./cmd/comandcenter

install:
	go install $(LDFLAGS) ./cmd/claudio
	go install $(LDFLAGS) ./cmd/comandcenter

test:
	go test ./...

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

# Rebuild, reinstall, and restart the running comandcenter + claudio --attach sessions.
#
# Prereq (one-time setup): create ~/.claudio/cc-start.sh with your start command, e.g.:
#   echo '#!/bin/sh\ncomandcenter --password YOUR_PASSWORD --port 8080' \
#     > ~/.claudio/cc-start.sh && chmod +x ~/.claudio/cc-start.sh
#
# claudio --attach sessions are killed so they restart with the new binary.
# If the session is run with a shell loop (while true; do ...; done) it restarts automatically.
# Otherwise restart it manually — it will reconnect to the already-running comandcenter.
CLAUDIO_DIR := $(HOME)/.claudio
CC_START    := $(CLAUDIO_DIR)/cc-start.sh

reload: install
	@echo "==> Restarting comandcenter..."
	@pkill -x comandcenter 2>/dev/null \
		&& echo "    killed comandcenter" \
		|| echo "    comandcenter was not running"
	@if [ -x "$(CC_START)" ]; then \
		nohup $(CC_START) >> $(CLAUDIO_DIR)/comandcenter.log 2>&1 & \
		echo "    started comandcenter (pid $$!) — logs: $(CLAUDIO_DIR)/comandcenter.log"; \
	else \
		echo ""; \
		echo "    !! No start script at $(CC_START)"; \
		echo "    !! Create it once:"; \
		echo "    !!   printf '#!/bin/sh\\ncomandcenter --password YOUR_PW --port 8080\\n' > $(CC_START) && chmod +x $(CC_START)"; \
		echo ""; \
	fi
	@echo "==> Restarting claudio --attach sessions..."
	@pkill -f "claudio.*--attach" 2>/dev/null \
		&& echo "    killed attach sessions — restart them (or they restart if in a shell loop)" \
		|| echo "    no attach sessions were running"
	@echo "==> Done."

# Cross-compilation
build-all:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/claudio-darwin-arm64 ./cmd/claudio
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/claudio-darwin-amd64 ./cmd/claudio
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/claudio-linux-amd64 ./cmd/claudio
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/claudio-linux-arm64 ./cmd/claudio
