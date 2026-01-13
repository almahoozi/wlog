.PHONY: all build build-tui clean run run-tui run-cli test install install-tui install-cli

LDFLAGS=-ldflags "-X main.commit=`git rev-parse HEAD` -X main.ref=`git rev-parse --abbrev-ref HEAD` -X main.version=`git describe --tags --always`"

all: clean test

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o ./bin/wlog ./cmd/cli

build-tui:
	CGO_ENABLED=0 go build $(LDFLAGS) -o ./bin/wlog-tui ./cmd/tui

clean:
	rm -rf ./bin

install:
	go install $(LDFLAGS) .

install-cli:
	go install $(LDFLAGS) ./cmd/cli

install-tui:
	go install $(LDFLAGS) ./cmd/tui

run:
	go run $(LDFLAGS) .

run-cli:
	go run $(LDFLAGS) ./cmd/cli

run-tui:
	go run $(LDFLAGS) ./cmd/tui

test:
	go test -v ./...
