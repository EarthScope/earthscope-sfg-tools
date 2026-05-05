# EarthScope Seafloor Geodesy Tools
[![Read the Docs](https://readthedocs.org/projects/es-sfgtools/badge/?version=latest)](https://es-sfgtools.readthedocs.io/en/latest/)

`es_sfgtools` is a Python library designed to support preprocessing and GNSS-A processing workflows for Seafloor Geodesy using data from Liquid Robotics SV2/SV3 Wave Gliders.

The toolkit also integrates with the [**GARPOS**](https://github.com/s-watanabe-jhod/garpos) GNSS-A processing.



## Installation

### Prerequisites

- [pixi](https://pixi.sh) (recommended) or conda/mamba

### Quick Start

```bash
git clone https://github.com/EarthScope/es_sfgtools.git
cd es_sfgtools

# Install environment and all packages
pixi install

# Build external dependencies (GARPOS, PRIDE-PPPAR, Go binaries)
pixi run setup

# Verify the setup
pixi run test-setup
```

### Development

```bash
# Lint and format
pixi run lint
pixi run format

# Run tests
pixi run pytest tests/ -v

# Build documentation
pixi run docs
```

## Documentation

Documentation (in development) is available on ReadTheDocs:

[ReadTheDocs](https://es-sfgtools.readthedocs.io/en/latest/)

---

**Maintainers**: Mike Gottlieb, Franklyn Dunbar, Rachel Akie
**Organization**: [EarthScope](https://www.earthscope.org/)