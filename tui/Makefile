BINARY = freedb
BUILD_DIR = build
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

.PHONY: build build-linux build-cross clean tidy test vet release

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) .

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 .

build-cross:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY)-$(GOOS)-$(GOARCH) .

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy

vet:
	go vet ./...

test: vet
	go test ./...

release: clean build-linux
	@echo ""
	@echo "Built: $(BUILD_DIR)/$(BINARY)-linux-amd64"
	@echo "Version: $(VERSION)"
	@echo ""
	@echo "To deploy:"
	@echo "  scp $(BUILD_DIR)/$(BINARY)-linux-amd64 HOST:/usr/local/bin/freedb"
	@echo ""
	@echo "To install on the host:"
	@echo "  sudo chmod +x /usr/local/bin/freedb"
	@echo "  sudo freedb check"
