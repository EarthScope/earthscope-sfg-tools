"""
Level 1 parsers for NovAtel logs, backed by the ``sfg nov2level1`` Go command.

Parsing uses the gnsstools NovAtel codecs (RANGEA, RANGECMP, RANGECMP5) and
the sfg_utils INSPVAA/INSSTDEVA deserializers. The Go command writes one
Arrow IPC file per Level 1 table holding only the parsed columns; this module
stamps scope and ``Log_Source`` and conforms each to its Iceberg table
definition in :mod:`earthscope_sfg_tools.datamodels.iceberg.schemas`.

One NOV000.bin file feeds three tables (GNSS observations, INSPVA,
INSSTDEV), so :func:`nov000_to_level1` returns all three from one parse.
"""

import logging
import subprocess
import tempfile
from pathlib import Path

import pyarrow as pa

from ..datamodels.iceberg.schemas import (
    GNSS_OBSERVATIONS,
    INSPVA,
    INSSTDEV,
    Scope,
    TableDef,
)
from ..utils.go_utils import find_binary, parse_cli_logs

logger = logging.getLogger(__name__)

_TABLES: dict[str, TableDef] = {
    t.name: t for t in (GNSS_OBSERVATIONS, INSPVA, INSSTDEV)
}


def _run_nov2level1(
    source: str | Path, fmt: str, antenna: int = 0
) -> dict[str, pa.Table]:
    """Run ``sfg nov2level1`` on one file; return its tables keyed by name."""
    source = Path(source)
    if not source.exists():
        raise FileNotFoundError(source)
    with tempfile.TemporaryDirectory() as out:
        cmd = [
            str(find_binary()),
            "nov2level1",
            "--format",
            fmt,
            "--antenna",
            str(antenna),
            "--out",
            out,
            str(source),
        ]
        result = subprocess.run(cmd, capture_output=True, text=True, check=False)
        parse_cli_logs(result, logger)
        if result.returncode != 0:
            raise RuntimeError(
                f"sfg nov2level1 failed on {source}: {result.stderr.strip()}"
            )
        tables = {}
        for path in Path(out).glob("*.arrow"):
            with pa.OSFile(str(path)) as f:
                tables[path.stem] = pa.ipc.open_file(f).read_all()
    return tables


def _conform(
    tables: dict[str, pa.Table], scope: Scope, log_source: str
) -> dict[str, pa.Table]:
    return {
        name: _TABLES[name].conform_arrow(table, scope, log_source)
        for name, table in tables.items()
    }


def nov000_to_level1(source: str | Path, scope: Scope) -> dict[str, pa.Table]:
    """Parse a NOV000.bin file into its ``gnss_observations``, ``inspva`` and
    ``insstdev`` Level 1 tables."""
    return _conform(_run_nov2level1(source, "nov000"), scope, "NOV000")


def nov770_to_gnss_observations(
    source: str | Path, scope: Scope, antenna: int = 0
) -> pa.Table:
    """Parse one antenna of a NOV770 (NovAtel binary RANGECMP/RANGECMP5) file.

    ``Log_Source`` is ``NOV770_ANT<antenna>`` so the antennas stay separate.
    """
    tables = _run_nov2level1(source, "novb", antenna)
    return _conform(tables, scope, f"NOV770_ANT{antenna}")["gnss_observations"]


def novatel_ascii_to_gnss_observations(source: str | Path, scope: Scope) -> pa.Table:
    """Parse a NovAtel ASCII (RANGEA) log into GNSS observations."""
    tables = _run_nov2level1(source, "nova")
    return _conform(tables, scope, "NOVATEL")["gnss_observations"]
