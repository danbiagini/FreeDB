BINARY = freedb
BUILD_DIR = build

.PHONY: build build-linux clean tidy

build:
	go build -o $(BUILD_DIR)/$(BINARY) .

build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY)-linux-amd64 .

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy

install: build-linux
	@echo "Copy $(BUILD_DIR)/$(BINARY)-linux-amd64 to your FreeDB host"
