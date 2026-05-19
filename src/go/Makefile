# Detect system OS and ARCH dynamically
OS := $(shell go env GOOS)
ARCH := $(shell go env GOARCH)

TEST_PREFIX := $(shell echo $$CONDA_PREFIX)

# Variables
BUILD_DIR=build
BINARY=sfg
# Use existing CONDA_PREFIX (from conda or micromamba); avoid calling `conda` directly
CONDA_PREFIX := $(shell echo $$CONDA_PREFIX)
# Optional override for CI or manual builds
SFG_TILEDB_PREFIX := $(shell echo $$SFG_TILEDB_PREFIX)

# Resolve TileDB prefix used for headers/libs
PREFIX := $(or $(SFG_TILEDB_PREFIX),$(CONDA_PREFIX))

# Derive build PREFIX:
#   On macOS: DYLD_LIBRARY_PATH takes priority when explicitly set (e.g. by the pixi
#             build-go task pointing at the tiledb env). Falls back to CONDA_PREFIX.
#   On Linux: same logic using LD_LIBRARY_PATH.
# This lets the pixi default environment build correctly without requiring -e tiledb.
ifeq ($(OS),darwin)
    DYLD_LIBRARY_PATH := $(PREFIX)/lib:$(DYLD_LIBRARY_PATH)
    # Use -Xlinker form to avoid Go CGO double-interpreting -Wl,-rpath,X
    RPATH_FLAG = -Xlinker -rpath -Xlinker @executable_path/libs
else
    LD_LIBRARY_PATH := $(PREFIX)/lib:$(LD_LIBRARY_PATH)
    RPATH_FLAG = -Xlinker -rpath -Xlinker $$ORIGIN/libs
endif

# CGO flags for TileDB
CGO_CFLAGS = -I$(PREFIX)/include -Wno-deprecated-declarations
CGO_LDFLAGS = -L$(PREFIX)/lib -ltiledb $(RPATH_FLAG)

LIBRARY_PATH = $(PREFIX)/lib
CPATH = $(PREFIX)/include
CLANG_PATH = $(PREFIX)/bin/clang
PSEUDO_PATH = $(CLANG_PATH)
ifeq ($(OS),darwin)
    CC = clang
else
    # Use the conda-forge gcc so CGO links against the conda sysroot (avoids
    # __res_search linker errors on Ubuntu 22.04+ glibc 2.34+).
    CC = $(CONDA_PREFIX)/bin/x86_64-conda-linux-gnu-gcc
    CGO_CFLAGS += --sysroot=$(CONDA_PREFIX)/x86_64-conda-linux-gnu/sysroot
    CGO_LDFLAGS += --sysroot=$(CONDA_PREFIX)/x86_64-conda-linux-gnu/sysroot
endif

# Build all commands into a single binary
# -buildvcs=false was added 8-21-25 to fix an issue building on AMD64 architecture
build:
	@mkdir -p $(BUILD_DIR)
	@BINARY_NAME=$(BINARY)_$(OS)_$(ARCH); \
	echo "Building $$BINARY_NAME for $(OS)/$(ARCH)..."; \
	env CC="$(CC)" CGO_ENABLED=1 GOOS=$(OS) GOARCH=$(ARCH) CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" DYLD_LIBRARY_PATH=$(DYLD_LIBRARY_PATH) LD_LIBRARY_PATH=$(LD_LIBRARY_PATH) go build -o $(BUILD_DIR)/$$BINARY_NAME -buildvcs=false ./cmd

# Bundle all binaries with their dylib dependencies so they run without conda.
# On macOS: copies libtiledb.dylib and its transitive deps into build/libs/,
#           rewrites all @rpath references to be self-contained, and strips the
#           absolute conda RPATH from each binary.
# On Linux: copies .so files into build/libs/ and uses patchelf to set RPATH.
#
# Requires: Python 3 (macOS); patchelf (Linux, install via conda: conda install patchelf)
bundle: build
ifeq ($(OS),darwin)
	@echo "Bundling dylibs for macOS..."
	@python3 $(CURDIR)/bundle_dylibs.py \
		--build-dir $(CURDIR)/$(BUILD_DIR) \
		--conda-lib $(PREFIX)/lib \
		--os $(OS) \
		--arch $(ARCH)
else
	@echo "Bundling .so files for Linux..."
	@python3 $(CURDIR)/bundle_dylibs.py \
		--build-dir $(CURDIR)/$(BUILD_DIR) \
		--conda-lib $(PREFIX)/lib \
		--os $(OS) \
		--arch $(ARCH)
endif
	@echo "Bundle complete. Contents:"
	@du -sh $(BUILD_DIR)/libs/
	@echo "Binaries are self-contained — no conda required at runtime."

debug:
	@echo "SHELL = $(SHELL)"
	@echo "OS: $(OS)"
	@echo "ARCH: $(ARCH)"
	@echo "TEST_PREFIX: $(TEST_PREFIX)"
	@echo "LIBRARY_PATH: $(LIBRARY_PATH)"
	@echo "Conda Prefix: $(CONDA_PREFIX)"
	@echo "Resolved PREFIX: $(PREFIX)"
	@echo "TileDB Library Path: $(PREFIX)/lib"
	@echo "DYLD_LIBRARY_PATH: $(DYLD_LIBRARY_PATH)"
	@echo "CPATH: $(CPATH)"
	@echo "Binary: $(BINARY)_$(OS)_$(ARCH)"