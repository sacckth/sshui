.PHONY: build test dist clean \
	package-linux-amd64 package-darwin-arm64 \
	pkg-deb pkg-rpm pkg-apk pkg-tar-darwin pkg-tar-linux \
	packages packages-all tag-push

BINARY := sshui
CMD := ./cmd/sshui/
VERSION ?= $(shell sed -n 's/^[[:space:]]*version[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' cmd/sshui/main.go | head -1)
ifeq ($(VERSION),)
  VERSION := 0.0.0
endif

# Git release tag (must match .github/workflows/release.yml: v*)
GIT_TAG := v$(VERSION)

# Static Linux binary for packages; embed version at link time.
LINUX_LDFLAGS := -s -w -X main.version=$(VERSION)
DARWIN_LDFLAGS := -s -w -X main.version=$(VERSION)

NFPM := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.41.2

# nfpm has no --version flag; inject VERSION into a generated config.
dist/nfpm-gen.yaml: nfpm.yaml
	mkdir -p dist
	sed 's/^version:.*/version: $(VERSION)/' nfpm.yaml > dist/nfpm-gen.yaml

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

# Legacy flat binaries in dist/
dist:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(DARWIN_LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 $(CMD)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LINUX_LDFLAGS)" -o dist/$(BINARY)-linux-amd64 $(CMD)

# Layout expected by nfpm (Linux amd64)
package-linux-amd64:
	mkdir -p dist/linux-amd64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LINUX_LDFLAGS)" -o dist/linux-amd64/$(BINARY) $(CMD)

# macOS Apple Silicon — tarball for distribution (not deb/rpm/apk)
package-darwin-arm64:
	mkdir -p dist/darwin-arm64
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(DARWIN_LDFLAGS)" -o dist/darwin-arm64/$(BINARY) $(CMD)

pkg-deb: package-linux-amd64 dist/nfpm-gen.yaml
	$(NFPM) package -f dist/nfpm-gen.yaml -p deb -t dist/

pkg-rpm: package-linux-amd64 dist/nfpm-gen.yaml
	$(NFPM) package -f dist/nfpm-gen.yaml -p rpm -t dist/

pkg-apk: package-linux-amd64 dist/nfpm-gen.yaml
	$(NFPM) package -f dist/nfpm-gen.yaml -p apk -t dist/

pkg-tar-darwin: package-darwin-arm64
	cd dist/darwin-arm64 && tar czf ../$(BINARY)-$(VERSION)-darwin-arm64.tar.gz $(BINARY)

pkg-tar-linux: package-linux-amd64
	cd dist/linux-amd64 && tar czf ../$(BINARY)-$(VERSION)-linux-amd64.tar.gz $(BINARY)

# All distributables: Linux packages + Linux/macOS tarballs
packages: pkg-deb pkg-rpm pkg-apk pkg-tar-darwin pkg-tar-linux
	@echo "Outputs under dist/: .deb .rpm .apk *.tar.gz"

packages-all: dist packages

clean:
	rm -rf dist $(BINARY)

# Create annotated tag v$(VERSION) from cmd/sshui/main.go and push to origin.
# Requires a clean working tree unless ALLOW_DIRTY=1. Fails if the tag exists locally.
tag-push:
	@git rev-parse --git-dir >/dev/null 2>&1 || { echo "error: not a git repository"; exit 1; }
ifndef ALLOW_DIRTY
	@test -z "$$(git status --porcelain)" || { echo "error: uncommitted changes (commit or stash, or run with ALLOW_DIRTY=1)"; exit 1; }
endif
	@git rev-parse $(GIT_TAG) >/dev/null 2>&1 && { echo "error: tag $(GIT_TAG) already exists"; exit 1; } || true
	git tag -a $(GIT_TAG) -m "Release $(GIT_TAG)"
	git push origin $(GIT_TAG)
	@echo "Pushed $(GIT_TAG)"
