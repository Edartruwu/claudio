BINARY = caido
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"
INSTALL_DIR = $(HOME)/.claudio/plugins
DIST_DIR = dist

PLATFORMS = \
  darwin/arm64 \
  darwin/amd64 \
  linux/amd64 \
  linux/arm64

.PHONY: build install build-all clean

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) .

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	chmod +x $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"

build-all:
	mkdir -p $(DIST_DIR)
	$(foreach platform,$(PLATFORMS), \
	  $(eval OS=$(word 1,$(subst /, ,$(platform)))) \
	  $(eval ARCH=$(word 2,$(subst /, ,$(platform)))) \
	  CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY)-$(OS)-$(ARCH) . ; \
	)

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)
