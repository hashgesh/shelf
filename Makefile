BINARY   := shelf
PKG      := ./cmd/shelf
DIST     := dist
STAGE    := build/release
TARGETS  := linux/amd64 linux/arm64 windows/amd64
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS  := -trimpath

export CGO_ENABLED := 0

.PHONY: all build test lint release clean

all: lint test build

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

test:
	go test ./...

lint:
	go vet ./...
	golangci-lint run ./...

# release builds every target into $(STAGE)/<os>_<arch>/ and packs them into
# $(DIST): .tar.gz for Linux, .zip for Windows, plus SHA256SUMS.
release: clean
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ "$$os" = windows ] && ext=.exe; \
		echo "build $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
			-o $(STAGE)/$${os}_$${arch}/$(BINARY)$$ext $(PKG) || exit 1; \
	done
	go run ./tools/pack -name $(BINARY) -version $(VERSION) -in $(STAGE) -out $(DIST)

clean:
	rm -rf bin build $(DIST)
