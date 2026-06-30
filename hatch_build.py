"""Hatchling build hook: compile the Go ``sfg`` binary before the wheel is built.

The wheel bundles ``src/go/build/sfg_*`` (see ``[tool.hatch.build.targets.wheel]``
in ``pyproject.toml``). Those binaries are the build *output* of ``src/go`` and are
not committed, so the wheel's ``artifacts`` glob matches nothing unless they are
compiled first. Previously that compile was a manual prerequisite
(``pixi run build-go``); a plain ``pip``/``uv install`` skipped it and silently
produced a binary-less wheel, which then failed at runtime in
``go_utils.find_binary``.

This hook runs ``make`` in ``src/go`` during the wheel build so the binary always
exists before hatchling collects artifacts. The binaries are CGO builds that link
TileDB, so a Go toolchain and TileDB headers/libs must be present at build time
(the pixi ``default``/``tiledb``/``geolab`` environments provide both). When the
toolchain is missing the hook fails loudly rather than shipping an empty wheel.
"""

import os
import shutil
import subprocess
from pathlib import Path

from hatchling.builders.hooks.plugin.interface import BuildHookInterface


class GoBuildHook(BuildHookInterface):
    """Compile the bundled Go binary into ``src/go/build`` before packaging."""

    PLUGIN_NAME = "custom"

    def initialize(self, version, build_data):
        # Only relevant for the wheel target (sdist ships source, not binaries).
        if self.target_name != "wheel":
            return

        root = Path(self.root)
        go_dir = root / "src" / "go"
        build_dir = go_dir / "build"

        if not (go_dir / "Makefile").exists():
            # Nothing to build (e.g. building from an sdist without the Go tree).
            return

        if shutil.which("go") is None:
            raise RuntimeError(
                "Go toolchain not found on PATH; cannot compile the `sfg` binary. "
                "Build inside an environment that provides Go + TileDB "
                "(e.g. `pixi install` then build), or install a prebuilt wheel."
            )

        self.app.display_info(f"Building Go `sfg` binary in {go_dir} ...")
        subprocess.run(
            ["make", "-B"],
            cwd=str(go_dir),
            check=True,
            env=os.environ.copy(),
        )

        built = sorted(build_dir.glob("sfg_*"))
        if not built:
            raise RuntimeError(
                f"`make` completed but produced no `sfg_*` binaries in {build_dir}. "
                "Check src/go/Makefile."
            )

        # Force-include the compiled binaries into the wheel. A plain `artifacts`
        # glob does not pull these in because they live outside the `packages`
        # root (src/go vs src/earthscope_sfg_tools); force_include copies them to
        # the in-package location that go_utils.find_binary() resolves.
        force_include = build_data.setdefault("force_include", {})
        for binary in built:
            force_include[str(binary)] = f"earthscope_sfg_tools/go/build/{binary.name}"

        self.app.display_info(
            "Bundled Go binaries: " + ", ".join(p.name for p in built)
        )
