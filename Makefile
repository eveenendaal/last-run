.PHONY: test build clean install

# RELEASE_VERSION (set by CI) wins, then the latest git tag, then "dev".
VERSION ?= $(or $(RELEASE_VERSION),$(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//'),dev)
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
