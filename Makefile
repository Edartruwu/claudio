VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X github.com/Abraxas-365/claudio/internal/cli.Version=$(VERSION)"

.PHONY: build run test clean install dev

dev: ## Run the development server with CSS watcher
	@[ -f node_modules/.bin/tailwindcss ] || npm install
	@trap 'kill 0' SIGINT SIGTERM; \
	./node_modules/.bin/tailwindcss -i internal/web/static/css/input.css -o internal/web/static/vendor/tailwind.min.css --watch & \
	go run $(LDFLAGS) ./cmd/claudio; \
	wait

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

.PHONY: css
css: ## Regenerate vendored Tailwind CSS from templates
	@[ -f node_modules/.bin/tailwindcss ] || npm install
	@./node_modules/.bin/tailwindcss \
		-i internal/web/static/css/input.css \
		-o internal/web/static/vendor/tailwind.min.css \
		--minify
	@echo "✅ Tailwind CSS regenerated"

.PHONY: css-watch
css-watch: ## Watch templates and auto-rebuild Tailwind CSS on change
	@[ -f node_modules/.bin/tailwindcss ] || npm install
	@./node_modules/.bin/tailwindcss \
		-i internal/web/static/css/input.css \
		-o internal/web/static/vendor/tailwind.min.css \
		--watch

# Cross-compilation
build-all:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/claudio-darwin-arm64 ./cmd/claudio
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/claudio-darwin-amd64 ./cmd/claudio
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/claudio-linux-amd64 ./cmd/claudio
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/claudio-linux-arm64 ./cmd/claudio
