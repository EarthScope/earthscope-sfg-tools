"""TileDB integration module.

Wraps the Go ``sfg`` binary subcommands that move NovAtel data in and out
of TileDB arrays, and re-exports the legacy TDB array classes and schemas.
Requires the ``tiledb`` extra: ``pip install earthscope-sfg-tools[tiledb]``.
"""

try:
    import tiledb  # noqa: F401
except ImportError as e:
    raise ImportError(
        "tiledb is required for earthscope_sfg_tools.tiledb_integration. "
        "Install it with: pip install earthscope-sfg-tools[tiledb]"
    ) from e

# Re-export legacy TDB array classes and schemas so workflow code that used
# `earthscope_sfg.tiledb_schemas` continues to work via the new module path.
from .arrays import (  # noqa: E402
    TBDArray,
    TDBAcousticArray,
    TDBGNSSObsArray,
    TDBIMUPositionArray,
    TDBKinPositionArray,
    TDBShotDataArray,
)
from .novatel_tools.go_binaries import (  # noqa: E402
    nov0002tile as _nov0002tile_impl,
    nova2tile,
    novb2tile,
    tdb2rnx,
)
from .schemas import (  # noqa: E402
    AcousticArraySchema,
    GNSSObsSchema,
    IMUPositionArraySchema,
    KinPositionArraySchema,
    ShotDataArraySchema,
    config,
    ctx,
    filters,
)


def novatel_770_2tile(files, gnss_obs_tdb, n_procs: int = 10, logger=None, **kwargs):
    """Workflow-facing wrapper around :func:`novb2tile` for NovAtel NOV770 logs.

    Translates the workflow-side kwargs (``files``, ``gnss_obs_tdb``,
    ``n_procs``) to the underlying ``sfg novab2tile`` call.
    """
    return novb2tile(
        input_files=files, tdb_path=gnss_obs_tdb, num_procs=n_procs, logger=logger
    )


def nov0002tile(
    files, gnss_obs_tdb, position_tdb=None, n_procs: int = 10, logger=None, **kwargs
):
    """Workflow-facing wrapper around the underlying ``sfg nov0002tile`` call.

    Translates the workflow-side kwargs (``files``, ``gnss_obs_tdb``,
    ``position_tdb``, ``n_procs``) to the binary's ``--tdb`` / ``--tdbpos`` /
    ``--procs`` flags.
    """
    return _nov0002tile_impl(
        input_files=files,
        tdb_path=gnss_obs_tdb,
        tdb_position=position_tdb,
        num_procs=n_procs,
        logger=logger,
    )


def _not_yet_implemented(name: str):
    """Build a placeholder callable for parser symbols not yet ported into
    earthscope-sfg-tools. These keep import-time compatibility for downstream
    consumers (e.g. earthscope-sfg-workflows) while the implementations are
    being moved over from the legacy ``earthscope_sfg`` package."""

    def _stub(*_args, **_kwargs):
        raise NotImplementedError(
            f"{name} has not yet been ported into earthscope_sfg_tools. "
            "Track progress in the migration plan."
        )

    _stub.__name__ = name
    _stub.__qualname__ = name
    return _stub


# TODO(earthscope-sfg-tools): implement these parser/converter helpers.
tile2rinex = _not_yet_implemented("tile2rinex")
rinex_qc = _not_yet_implemented("rinex_qc")


__all__ = [
    # Go binary wrappers
    "nova2tile",
    "novb2tile",
    "nov0002tile",
    "novatel_770_2tile",
    "tdb2rnx",
    # Not yet ported (import-compatible stubs)
    "tile2rinex",
    "rinex_qc",
    # Legacy-compatible TDB array classes
    "TBDArray",
    "TDBAcousticArray",
    "TDBGNSSObsArray",
    "TDBIMUPositionArray",
    "TDBKinPositionArray",
    "TDBShotDataArray",
    # Schema symbols
    "AcousticArraySchema",
    "GNSSObsSchema",
    "IMUPositionArraySchema",
    "KinPositionArraySchema",
    "ShotDataArraySchema",
    "config",
    "ctx",
    "filters",
]
