.PHONY: all build build-tui clean run test install

LDFLAGS=-ldflags "-X main.commit=`git rev-parse HEAD` -X main.ref=`git rev-parse --abbrev-ref HEAD` -X main.version=`git describe --tags --always`"

all: clean test

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o ./bin/wlog ./cmd/cli

build-tui:
	CGO_ENABLED=0 go build $(LDFLAGS) -o ./bin/wlog-tui ./cmd/tui

clean:
	rm -rf ./bin

install:
	@echo "Installing wlog..."
	@CGO_ENABLED=0 go install $(LDFLAGS) .
	@wlog version
	@echo "wlog installed successfully to $(GOPATH)/bin/wlog"

run:
	CGO_ENABLED=0 go run $(LDFLAGS) .

test:
	go test -v ./...
