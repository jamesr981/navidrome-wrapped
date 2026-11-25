PLUGIN       := navidrome-wrapped
# Navidrome release whose Go PDK we build against (needs 0.64+ for the scrobble retriever API)
NAVIDROME_REF ?= v0.64.2

.PHONY: all build test pdk clean

all: $(PLUGIN).ndp

# Fetch the Navidrome Go PDK (it isn't published as a standalone tagged module)
pdk: .pdk/plugins/pdk/go/go.mod
.pdk/plugins/pdk/go/go.mod:
	git clone --depth 1 --branch $(NAVIDROME_REF) --filter=blob:none --sparse https://github.com/navidrome/navidrome.git .pdk
	cd .pdk && git sparse-checkout set --no-cone /plugins/pdk/go/

plugin.wasm: pdk *.go story.html index.html go.mod
	tinygo build -o plugin.wasm -target wasip1 -buildmode=c-shared .

$(PLUGIN).ndp: plugin.wasm manifest.json
	rm -f $@
	zip -j $@ manifest.json plugin.wasm

build: $(PLUGIN).ndp

test: pdk
	go test ./...

clean:
	rm -f plugin.wasm $(PLUGIN).ndp
