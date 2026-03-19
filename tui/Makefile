BINARY = freedb
BUILD_DIR = build
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

.PHONY: build build-linux clean tidy

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) .

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 .

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy

install: build-linux
	@echo "Copy $(BUILD_DIR)/$(BINARY)-linux-amd64 to your FreeDB host"
	@echo "Version: $(VERSION)"
