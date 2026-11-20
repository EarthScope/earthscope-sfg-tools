"""
Level 0 parsers for NovAtel logs, backed by ``sfg convert --source raw``.

The Go side parses the file, stamps ``stream_edid`` with the scope and log
source, and writes one Parquet file per Level 0 table to
``<out>/gnssa_l0/<table>/``. This module reads those files back as Arrow
tables keyed by table name, with the columns Go wrote (the Go table
definitions in ``src/go/pkg/datamodels``); nothing is re-stamped or
re-conformed here.

One NOV000.bin file feeds three tables (GNSS observations, INSPVA,
INSSTDEV), so :func:`nov000_to_level0` returns every table the file has rows
for from one parse. A table with no rows is not written by Go and so is
absent from the result.
"""

import logging
import subprocess
import tempfile
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq

from ..datamodels.iceberg.schemas import Scope
from ..utils.go_utils import find_binary, parse_cli_logs

logger = logging.getLogger(__name__)

_DATABASE = "gnssa_l0"
_GNSS_OBSERVATIONS = "gnss_observations"

# The gnss_observations columns Go writes, from its table definition
# (src/go/pkg/datamodels, TableDef.ArrowSchema): the schema of an empty
# result, since Go writes no file for a table without rows. Keep in step with
# the Go definition.
_UTC_US = pa.timestamp("us", tz="UTC")
GNSS_OBSERVATIONS_SCHEMA = pa.schema(
    [
        pa.field("stream_edid", pa.string(), nullable=False),
        pa.field("time_utc", _UTC_US, nullable=False),
        pa.field("time_gpst", _UTC_US, nullable=False),
        pa.field("time_sub_us_ps", pa.int32(), nullable=False),
        pa.field("clock_offset", pa.float64()),
        pa.field("constellation", pa.string(), nullable=False),
        pa.field("satellite_id", pa.int32(), nullable=False),
        pa.field("signal", pa.string(), nullable=False),
        pa.field("code", pa.float64()),
        pa.field("phase", pa.float64()),
        pa.field("doppler", pa.float64()),
        pa.field("snr", pa.float64()),
        pa.field("loss_of_lock", pa.bool_()),
        pa.field("half_cycle_ambiguity", pa.bool_()),
        pa.field("boc_tracking", pa.bool_()),
        pa.field("lock_time", pa.float64()),
        pa.field("epoch_flag", pa.int32()),
        pa.field("fcn", pa.int32()),
        pa.field("write_time", _UTC_US),
        pa.field("arrival_time", _UTC_US),
    ]
)

# NOV770 log sources for an antenna: the primary antenna has the bare name, the
# second one the _1 suffix (edid.AntennaLogName); a file holds RANGECMPB or
# RANGECMP5B, so both are accepted.
_NOV770_NAMES = {
    0: ("RANGECMPB", "RANGECMP5B"),
    1: ("RANGECMPB_1", "RANGECMP5B_1"),
}


def _run_level0(
    source: str | Path,
    file_type: str,
    scope: Scope,
    log_source: str | None = None,
) -> dict[str, pa.Table]:
    """Run ``sfg convert --source raw --sink files`` on one file; return the
    ``gnssa_l0`` tables it wrote keyed by name.

    ``log_source`` is ``<FILE_TYPE>.<NAME>`` (e.g. ``NOV770.RANGECMP5B_1``) to
    keep one stream's rows, or None for every stream of ``file_type``.
    """
    source = Path(source)
    if not source.exists():
        raise FileNotFoundError(source)
    with tempfile.TemporaryDirectory() as out:
        cmd = [
            str(find_binary()),
            "convert",
            "--source",
            "raw",
            "--source-uri",
            str(source),
            "--source-type",
            file_type,
            "--sink",
            "files",
            "--table-format",
            "parquet",
            "--sink-uri",
            out,
            "--scope",
            f"{scope.network}/{scope.station}/{scope.campaign}",
        ]
        if log_source:
            cmd += ["--log-source", log_source]
        result = subprocess.run(cmd, capture_output=True, text=True, check=False)
        parse_cli_logs(result, logger)
        if result.returncode != 0:
            raise RuntimeError(
                f"sfg convert failed on {source} (exit {result.returncode}): "
                f"{result.stderr.strip()}"
            )
        tables = {}
        for table_dir in sorted((Path(out) / _DATABASE).glob("*")):
            parts = [pq.read_table(p) for p in sorted(table_dir.glob("*.parquet"))]
            if parts:
                tables[table_dir.name] = pa.concat_tables(parts)
    return tables


def nov000_to_level0(source: str | Path, scope: Scope) -> dict[str, pa.Table]:
    """Parse a NOV000.bin file into its ``gnss_observations``, ``inspva`` and
    ``insstdev`` Level 0 tables (those with rows)."""
    return _run_level0(source, "NOV000", scope)


def nov770_to_gnss_observations(
    source: str | Path, scope: Scope, antenna: int = 0
) -> pa.Table:
    """Parse one antenna of a NOV770 (NovAtel binary RANGECMP/RANGECMP5) file.

    The antennas stay separate through ``stream_edid``: the primary antenna's
    log source is ``NOV770.RANGECMPB`` or ``NOV770.RANGECMP5B`` and the second
    antenna's has the ``_1`` suffix. Returns an empty table with the
    ``gnss_observations`` columns (:data:`GNSS_OBSERVATIONS_SCHEMA`) if the
    file has no rows for that antenna.
    """
    if antenna not in _NOV770_NAMES:
        raise ValueError(f"antenna {antenna}: want 0 or 1")
    table = _run_level0(source, "NOV770", scope).get(_GNSS_OBSERVATIONS)
    if table is None:
        return GNSS_OBSERVATIONS_SCHEMA.empty_table()
    suffixes = tuple(f".NOV770.{name}" for name in _NOV770_NAMES[antenna])
    keep = [s.endswith(suffixes) for s in table["stream_edid"].to_pylist()]
    return table.filter(pa.array(keep, pa.bool_()))


def novatel_ascii_to_gnss_observations(source: str | Path, scope: Scope) -> pa.Table:
    """Parse a NovAtel ASCII (RANGEA) log into GNSS observations.

    Returns an empty table with the ``gnss_observations`` columns
    (:data:`GNSS_OBSERVATIONS_SCHEMA`) if the file has no rows.
    """
    tables = _run_level0(source, "NVA", scope, log_source="NVA.RANGEA")
    table = tables.get(_GNSS_OBSERVATIONS)
    if table is None:
        return GNSS_OBSERVATIONS_SCHEMA.empty_table()
    return table
