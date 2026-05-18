# Development

## Common tasks

All tasks are defined in `pyproject.toml` and run via `pixi run`:

```bash
pixi run test           # Run the test suite (pytest)
pixi run lint           # Lint with ruff
pixi run format-check   # Check formatting (ruff format --check)
pixi run format         # Auto-format source
pixi run build-go       # Build Go binaries
```

List all available tasks:

```bash
pixi task list
```

## Running a task in a specific environment

```bash
pixi run -e tiledb build-go
pixi run -e geolab python -c "import tiledb; print(tiledb.__version__)"
```

## Managing dependencies

```bash
# Add a conda package
pixi add numpy

# Add a package to a specific feature/environment
pixi add --feature tiledb tiledb

# Add a PyPI package
pixi add --pypi requests

# After changing pyproject.toml dependencies, re-solve:
pixi install
```

## Pre-commit hooks

This project uses [ruff](https://docs.astral.sh/ruff/) via pre-commit for
linting and formatting. Install hooks once after cloning:

```bash
pixi run -e default pre-commit install
```

Hooks run automatically on `git commit`. To run manually:

```bash
pixi run lint
pixi run format-check
```

## CI

GitHub Actions runs on push/PR to `main` when `src/`, `tests/`, or
`pyproject.toml` change:

1. Ruff format check
2. Build Go binaries (in `tiledb` environment)
3. `pytest` test suite (in `tiledb` environment)
