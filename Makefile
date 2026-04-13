VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
LDFLAGS  = -X github.com/alpha-code-labs/schemaguard/internal/cli.Version=$(VERSION)

.PHONY: build install test vet clean

build:
	go build -ldflags '$(LDFLAGS)' -o schemaguard ./cmd/schemaguard

install:
	go install -ldflags '$(LDFLAGS)' ./cmd/schemaguard

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f schemaguard
