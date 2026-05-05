# PIXI Guide

This project uses [pixi](https://pixi.sh) for environment and task management.

## 1) Install pixi

Choose one method:

### macOS (Homebrew)
```bash
brew install pixi
```

### macOS / Linux (install script)
```bash
curl -fsSL https://pixi.sh/install.sh | bash
```

### Verify installation
```bash
pixi --version
```

## 2) Set up this project

From the repository root:

```bash
pixi install
```

This creates the project environments in `.pixi/envs/`.

## 3) Activate an environment

### Default environment
```bash
pixi shell
```

### Named environments in this repo
- `default`
- `tiledb`
- `geolab`

Activate a specific one:

```bash
pixi shell -e tiledb
```

Exit the shell:

```bash
exit
```

## 4) Run pixi tasks

List available tasks:

```bash
pixi task list
```

Run tasks defined in `pyproject.toml`:

```bash
pixi run test
pixi run lint
pixi run format-check
pixi run format
pixi run build-go
```

Run a task in a specific environment:

```bash
pixi run -e tiledb build-go
```

## 5) Run code with pixi

You can run commands without activating a shell.

### Run Python one-off commands
```bash
pixi run python -c "import earthscope_sfg_tools; print('ok')"
```

### Run a Python file
```bash
pixi run python path/to/script.py
```

### Run code in a specific environment
```bash
pixi run -e geolab python -c "import tiledb; print(tiledb.__version__)"
```

## 6) Helpful tips

- Use `pixi shell -e <env-name>` for interactive work.
- Use `pixi run ...` for reproducible, one-shot commands (great for CI and scripts).
- If dependencies change in `pyproject.toml`, run `pixi install` again.

## 7) Development with pixi

This section covers common workflows when you are actively developing.

### Add packages

#### Add a conda package to the default environment
```bash
pixi add numpy
```

#### Add a conda package to a feature/environment
```bash
pixi add --feature tiledb tiledb
```

#### Add a PyPI package
```bash
pixi add --pypi requests
```

#### Add a development-only dependency
```bash
pixi add --feature dev pytest-cov
```

### Remove packages

```bash
pixi remove numpy
pixi remove --feature tiledb tiledb
pixi remove --pypi requests
```

### Add or update tasks

You can define tasks in `pyproject.toml` under `[tool.pixi.tasks]`, then run them with `pixi run <task-name>`.

Example:
```toml
[tool.pixi.tasks]
check = { cmd = "ruff check src/ tests/", description = "Run lint checks" }
```

Run it:
```bash
pixi run check
```

### Lock/install/update dependencies

```bash
# Sync/install what is declared in pyproject + lock file
pixi install

# Update lock file and solve to newer compatible versions
pixi update
```

### Create a new pixi workspace (new project)

If you are starting a brand-new repository:

```bash
mkdir my-project && cd my-project
pixi init
pixi add python
pixi add --pypi pytest
```

Then add tasks in `pyproject.toml`:

```toml
[tool.pixi.tasks]
test = { cmd = "pytest -v", description = "Run tests" }
```

Run the task:
```bash
pixi run test
```

### Create additional named environments in this repo

In this repository, named environments are configured in `pyproject.toml` under `[tool.pixi.environments]` and built from features.

Pattern:
1. Add feature dependencies under `[tool.pixi.feature.<feature-name>.dependencies]` and/or `[tool.pixi.feature.<feature-name>.pypi-dependencies]`.
2. Register an environment under `[tool.pixi.environments]` using that feature.
3. Run `pixi install`.
4. Use it with `pixi shell -e <env-name>` or `pixi run -e <env-name> ...`.
