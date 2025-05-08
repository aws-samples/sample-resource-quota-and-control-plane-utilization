# Makefile

# Name of your extension executable under /opt/extensions
EXT_NAME   := emf

# Where to drop the compiled assets
BUILD_DIR  := build
EXT_DIR    := $(BUILD_DIR)/extensions
BIN_PATH   := $(EXT_DIR)/$(EXT_NAME)

# Path to your extension’s main.go
SRC        := cmd/emf-extension/main.go

.PHONY: all build package clean

all: package

# 1) Build the Linux/ARM64 binary under build/extensions/emf
build:
	mkdir -p $(EXT_DIR)
	GOOS=linux GOARCH=arm64 go build -o $(BIN_PATH) $(SRC)
	chmod +x $(BIN_PATH)

# 2) Zip up the extensions/ tree so it contains:
#    extensions/
#    └── emf   ← your executable
package: build
	cd $(BUILD_DIR) && zip -r ../emf-extension.zip extensions

clean:
	rm -rf $(BUILD_DIR) *.zip
