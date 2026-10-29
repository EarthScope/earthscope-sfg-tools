# Detect system OS and ARCH dynamically
OS := $(shell go env GOOS)
ARCH := $(shell go env GOARCH)

# Variables
BUILD_DIR=build
BINARY=sfg

# build names a target, not the build/ directory it writes into.
.PHONY: build debug

# Build all commands into a single binary. The binary is pure Go (no CGO), so
# it needs no C toolchain or shared libraries at build or run time.
# -buildvcs=false was added 8-21-25 to fix an issue building on AMD64 architecture
build:
	@mkdir -p $(BUILD_DIR)
	@BINARY_NAME=$(BINARY)_$(OS)_$(ARCH); \
	echo "Building $$BINARY_NAME for $(OS)/$(ARCH)..."; \
	env CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -o $(BUILD_DIR)/$$BINARY_NAME -buildvcs=false ./cmd

debug:
	@echo "SHELL = $(SHELL)"
	@echo "OS: $(OS)"
	@echo "ARCH: $(ARCH)"
	@echo "Binary: $(BINARY)_$(OS)_$(ARCH)"
