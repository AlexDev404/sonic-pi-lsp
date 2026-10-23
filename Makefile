# Day-to-day tasks. The release packages are built by CI
# (.github/workflows/release.yml) with the scripts in scripts/.

VERSION ?= dev
LDFLAGS := -s -w -X github.com/AlexDev404/sonic-pi-lsp/internal/version.Version=$(VERSION)

.PHONY: build test server package lint-docs clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o build/bin/ ./cmd/sonicpi ./cmd/sonicpid

test:
	go vet ./...
	go test ./...
	ruby -EUTF-8 lint/test/test_splint.rb

# The Sonic Pi server, from the submodule (Linux).
server:
	scripts/build-server-linux.sh

package: build
	scripts/package-linux.sh $(VERSION)

lint-docs:
	ruby -EUTF-8 scripts/gen-lint-docs.rb > docs/lint-rules.md

clean:
	rm -rf build dist
