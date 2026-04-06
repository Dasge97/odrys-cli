GO ?= $(HOME)/.local/go/bin/go
GOCACHE_DIR ?= $(CURDIR)/.go-cache
GOMODCACHE_DIR ?= $(CURDIR)/.go-mod-cache
GOPATH_DIR ?= $(CURDIR)/.go-path

GO_RUN = mkdir -p dist $(GOCACHE_DIR) $(GOMODCACHE_DIR) $(GOPATH_DIR) && GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" GOPATH="$(GOPATH_DIR)" $(GO)

run:
	$(GO_RUN) run ./cmd/odrys

build:
	$(GO_RUN) build -o dist/odrys ./cmd/odrys

run-server:
	$(GO_RUN) run ./cmd/odrys-core

build-server:
	$(GO_RUN) build -o dist/odrys-core ./cmd/odrys-core
