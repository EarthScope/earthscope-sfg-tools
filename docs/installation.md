# Installation

## Prerequisites

- [pixi](https://pixi.sh) — used for environment and task management
- Python 3.11+
- Go 1.25+ *(only required if building the optional Go binaries)*

### Install pixi

::::{tab-set}
:::{tab-item} macOS (Homebrew)
```bash
brew install pixi
```
:::
:::{tab-item} macOS / Linux
```bash
curl -fsSL https://pixi.sh/install.sh | bash
```
:::
::::

## Set up the project

Clone the repository and install all dependencies:

```bash
git clone https://github.com/EarthScope/earthscope-sfg-tools.git
cd earthscope-sfg-tools
pixi install
```

This creates isolated environments under `.pixi/envs/`.

## Environments

Three environments are available depending on what you need:

| Environment | Includes | Use when |
|-------------|----------|----------|
| `default` | Core Python deps | General development and testing |
| `tiledb` | + TileDB | Working with TileDB storage |
| `geolab` | + TileDB + Jupyter | Interactive exploration |

Activate a specific environment:

```bash
pixi shell -e tiledb
```

## Optional: build Go binaries

High-performance CLI tools (NovAtel → RINEX/TileDB conversion, RINEX QC) are
written in Go and wrapped by the Python package. Build them with:

```bash
pixi run build-go
```

This requires Go 1.25+ and runs `make -B` inside `src/go/`.

## Verify the installation

```bash
pixi run test
```
