.PHONY: test build clean install

VERSION ?= $(shell echo "$${RELEASE_VERSION:-$$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)}")
LDFLAGS = -s -w -X main.version=$(VERSION)

test:
	go test ./...

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/lastrun ./cmd/lastrun
	(cd dist && sha256sum lastrun > lastrun.sha256)

clean:
	rm -rf dist

install:
	@GOBIN="$$(go env GOBIN)"; \
	[ -z "$$GOBIN" ] && GOBIN="$$(go env GOPATH)/bin"; \
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o "$$GOBIN/lastrun" ./cmd/lastrun
