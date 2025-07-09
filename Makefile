# Makefile

# Name of your extension executable under /opt/extensions
EXT_NAME   := emf

# Where to drop the compiled assets
BUILD_DIR  := build
EXT_DIR    := $(BUILD_DIR)/extensions
BIN_PATH   := $(EXT_DIR)/$(EXT_NAME)

# Path to your extension’s main.go
SRC        := cmd/emf-extension/main.go

# S3 upload settings
BUCKET ?=                # must be set when running 'upload'
KEY    := emf-extension.zip

.PHONY: all build package upload clean

# default: build, package, then upload to S3
all: package upload

# 1) Build the Linux/ARM64 binary under build/extensions/emf
build:
	mkdir -p $(EXT_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(BIN_PATH) $(SRC)
	chmod +x $(BIN_PATH)

# 2) Zip up the extensions/ tree so it contains:
#    extensions/
#    └── emf   ← your executable
package: build
	cd $(BUILD_DIR) && zip -r ../$(KEY) extensions

# 3) Upload the zip to S3
upload: package
ifndef BUCKET
	$(error BUCKET is not set; usage: make upload BUCKET=<your-bucket>)
endif
	aws s3 cp $(KEY) s3://$(BUCKET)/emf/$(KEY)

# clean up everything we generated
clean:
	rm -rf $(BUILD_DIR) *.zip

