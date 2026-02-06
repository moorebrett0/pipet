BINARY  := pipet
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build release clean

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/pipet

release:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-arm64 ./cmd/pipet
	GOOS=linux GOARCH=arm   CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-arm   ./cmd/pipet

clean:
	rm -f $(BINARY) $(BINARY)-linux-*

vet:
	go vet ./...
