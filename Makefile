BINARY := kumo
PKG := github.com/kumobase/kumo-cli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION)

.PHONY: build run test vet lint tidy install clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -f $(BINARY)
