PLUGIN       := navidrome-wrapped
# Navidrome release whose Go PDK we build against (needs 0.64+ for the scrobble retriever API)
NAVIDROME_REF ?= v0.64.2

# Plugin version: taken from a v* tag on the current commit (v1.2.3 -> 1.2.3), otherwise a dev build.
# Override with `make VERSION=1.2.3`.
VERSION ?= $(shell git describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null)
PLUGIN_VERSION := $(patsubst v%,%,$(VERSION))
ifeq ($(PLUGIN_VERSION),)
PLUGIN_VERSION := 0.0.0-dev+$(shell git rev-parse --short HEAD 2>/dev/null || echo local)
endif

.PHONY: all build test pdk clean FORCE

all: $(PLUGIN).ndp

# Fetch the Navidrome Go PDK (it isn't published as a standalone tagged module)
pdk: .pdk/plugins/pdk/go/go.mod
.pdk/plugins/pdk/go/go.mod:
	git clone --depth 1 --branch $(NAVIDROME_REF) --filter=blob:none --sparse https://github.com/navidrome/navidrome.git .pdk
	cd .pdk && git sparse-checkout set --no-cone /plugins/pdk/go/

# Only touched when the version changes, so a new tag triggers a rebuild
.version: FORCE
	@echo '$(PLUGIN_VERSION)' | cmp -s - $@ || echo '$(PLUGIN_VERSION)' > $@

plugin.wasm: pdk *.go story.html index.html go.mod .version
	tinygo build -o plugin.wasm -target wasip1 -buildmode=c-shared -ldflags "-X main.version=$(PLUGIN_VERSION)" .

$(PLUGIN).ndp: plugin.wasm manifest.json .version
	rm -rf $@ build
	mkdir -p build
	sed 's/^\(  "version": \)"[^"]*"/\1"$(PLUGIN_VERSION)"/' manifest.json > build/manifest.json
	zip -j $@ build/manifest.json plugin.wasm

build: $(PLUGIN).ndp

test: pdk
	go test ./...

clean:
	rm -rf plugin.wasm $(PLUGIN).ndp build .version
