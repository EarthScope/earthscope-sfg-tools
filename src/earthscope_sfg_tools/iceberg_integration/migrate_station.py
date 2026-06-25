"""
One-time migration script: TileDB arrays → Iceberg tables for a single station.

Usage
-----
    python -m earthscope_sfg_tools.iceberg_integration.migrate_station \\
        --tdb-root  /path/to/station/TileDB \\
        --network   SEAFLOOR \\
        --station   GCC1 \\
        --campaign  2024-CRUISE \\
        --catalog-mode  local \\
        --warehouse /path/to/iceberg/warehouse

    # S3 + Glue (production)
    python -m earthscope_sfg_tools.iceberg_integration.migrate_station \\
        --tdb-root  s3://bucket/stations/GCC1/TileDB \\
        --network   SEAFLOOR \\
        --station   GCC1 \\
        --campaign  2024-CRUISE \\
        --catalog-mode  glue

Flags
-----
    --dry-run       Print what would be migrated without writing anything.
    --arrays        Comma-separated list of arrays to migrate (default: all).
                    Choices: kin_position, imu_position, acoustic, shotdata,
                             shotdata_pre, gnss_obs, gnss_obs_secondary,
                             qc_kin_position, qc_shotdata, qc_shotdata_pre,
                             qc_gnss_obs

Notes
-----
- Original .tdb files are never deleted or modified.
- The script is idempotent: re-running on a station that is already migrated
  appends duplicate rows. Use --dry-run first to verify state.
- Row counts are validated after each array; a mismatch exits with code 1.
"""

from __future__ import annotations

import argparse
import logging
import sys
from dataclasses import dataclass
from pathlib import Path

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Array registry
# ---------------------------------------------------------------------------

_ALL_ARRAYS = [
    "kin_position",
    "imu_position",
    "acoustic",
    "shotdata",
    "shotdata_pre",
    "gnss_obs",
    "gnss_obs_secondary",
    "qc_kin_position",
    "qc_shotdata",
    "qc_shotdata_pre",
    "qc_gnss_obs",
]

# Maps array name → (tdb filename, iceberg array class name)
_TDB_FILENAMES: dict[str, str] = {
    "kin_position": "kin_position.tdb",
    "imu_position": "imu_position.tdb",
    "acoustic": "acoustic.tdb",
    "shotdata": "shotdata.tdb",
    "shotdata_pre": "shotdata_pre.tdb",
    "gnss_obs": "gnss_obs.tdb",
    "gnss_obs_secondary": "gnss_obs_secondary.tdb",
    "qc_kin_position": "qc_kin_position.tdb",
    "qc_shotdata": "qc_shotdata.tdb",
    "qc_shotdata_pre": "qc_shotdata_pre.tdb",
    "qc_gnss_obs": "qc_gnss_obs.tdb",
}

# Maps array name → Iceberg table name (qc variants share the same table,
# differentiated by the campaign column if needed — or use separate tables)
_ICEBERG_TABLE_NAMES: dict[str, str] = {
    "kin_position": "kin_position",
    "qc_kin_position": "qc_kin_position",
    "imu_position": "imu_position",
    "acoustic": "acoustic",
    "shotdata": "shotdata",
    "shotdata_pre": "shotdata_pre",
    "gnss_obs": "gnss_obs",
    "gnss_obs_secondary": "gnss_obs_secondary",
    "qc_shotdata": "qc_shotdata",
    "qc_shotdata_pre": "qc_shotdata_pre",
    "qc_gnss_obs": "qc_gnss_obs",
}


@dataclass
class MigrationResult:
    array_name: str
    tdb_rows: int
    iceberg_rows: int
    success: bool
    error: str = ""

    def __str__(self) -> str:
        status = "OK" if self.success else "FAIL"
        if self.error:
            return f"[{status}] {self.array_name}: {self.error}"
        return f"[{status}] {self.array_name}: {self.tdb_rows} rows → {self.iceberg_rows} rows"


# ---------------------------------------------------------------------------
# Core migration logic
# ---------------------------------------------------------------------------


def _get_tdb_class(array_name: str):
    """Return the TileDB array class for the given array name."""
    from earthscope_sfg_tools.tiledb_integration.arrays import (
        TDBAcousticArray,
        TDBGNSSObsArray,
        TDBIMUPositionArray,
        TDBKinPositionArray,
        TDBShotDataArray,
    )

    mapping = {
        "kin_position": TDBKinPositionArray,
        "qc_kin_position": TDBKinPositionArray,
        "imu_position": TDBIMUPositionArray,
        "acoustic": TDBAcousticArray,
        "shotdata": TDBShotDataArray,
        "shotdata_pre": TDBShotDataArray,
        "gnss_obs": TDBGNSSObsArray,
        "gnss_obs_secondary": TDBGNSSObsArray,
        "qc_shotdata": TDBShotDataArray,
        "qc_shotdata_pre": TDBShotDataArray,
        "qc_gnss_obs": TDBGNSSObsArray,
    }
    cls = mapping.get(array_name)
    if cls is None:
        raise ValueError(f"Unknown array name: {array_name!r}")
    return cls


def _get_iceberg_class(array_name: str):
    """Return the Iceberg array class for the given array name."""
    from earthscope_sfg_tools.iceberg_integration.arrays import (
        IcebergAcousticArray,
        IcebergGNSSObsArray,
        IcebergIMUPositionArray,
        IcebergKinPositionArray,
        IcebergShotDataArray,
    )
    from earthscope_sfg_tools.iceberg_integration.schemas import (
        AcousticPartitionSpec,
        AcousticSchema,
        GNSSObsPartitionSpec,
        GNSSObsSchema,
        IMUPositionPartitionSpec,
        IMUPositionSchema,
        KinPositionPartitionSpec,
        KinPositionSchema,
        ShotDataPartitionSpec,
        ShotDataSchema,
    )

    # QC arrays use the same Iceberg classes but different table names, so we
    # return a (class, override_table_name) pair.
    mapping = {
        "kin_position": (IcebergKinPositionArray, None),
        "qc_kin_position": (IcebergKinPositionArray, "qc_kin_position"),
        "imu_position": (IcebergIMUPositionArray, None),
        "acoustic": (IcebergAcousticArray, None),
        "shotdata": (IcebergShotDataArray, None),
        "shotdata_pre": (IcebergShotDataArray, "shotdata_pre"),
        "gnss_obs": (IcebergGNSSObsArray, None),
        "gnss_obs_secondary": (IcebergGNSSObsArray, "gnss_obs_secondary"),
        "qc_shotdata": (IcebergShotDataArray, "qc_shotdata"),
        "qc_shotdata_pre": (IcebergShotDataArray, "qc_shotdata_pre"),
        "qc_gnss_obs": (IcebergGNSSObsArray, "qc_gnss_obs"),
    }
    entry = mapping.get(array_name)
    if entry is None:
        raise ValueError(f"Unknown array name: {array_name!r}")
    return entry


def _read_all_from_tdb(tdb_array, array_name: str):
    """Read all data from a TileDB array. Returns a DataFrame or list of rows."""
    import datetime

    import numpy as np
    import tiledb

    uri = str(tdb_array.uri)
    if not tiledb.array_exists(uri):
        logger.warning("TileDB array does not exist: %s", uri)
        return None

    # Read across a wide date range to capture all data
    epoch_start = datetime.datetime(1980, 1, 1, tzinfo=datetime.timezone.utc)
    epoch_end = datetime.datetime(2100, 1, 1, tzinfo=datetime.timezone.utc)

    try:
        df = tdb_array.read_df(epoch_start, epoch_end, validate=False)
        return df
    except Exception as exc:
        logger.error("Failed to read from TileDB %s: %s", array_name, exc)
        return None


def migrate_array(
    array_name: str,
    tdb_root: str,
    catalog,
    scope,
    dry_run: bool = False,
) -> MigrationResult:
    """Migrate a single TileDB array to its Iceberg equivalent."""
    import pandas as pd

    tdb_filename = _TDB_FILENAMES[array_name]
    tdb_uri = str(Path(tdb_root) / tdb_filename)

    logger.info("Migrating %s (tdb=%s)", array_name, tdb_uri)

    # --- Load TileDB array ---
    try:
        tdb_cls = _get_tdb_class(array_name)
        tdb_arr = tdb_cls(tdb_uri)
    except Exception as exc:
        return MigrationResult(array_name, 0, 0, False, f"TileDB init failed: {exc}")

    # --- Read all data from TileDB ---
    df = _read_all_from_tdb(tdb_arr, array_name)
    if df is None or (hasattr(df, "empty") and df.empty):
        logger.info("  No data found in %s, skipping", array_name)
        return MigrationResult(array_name, 0, 0, True, "no data")

    tdb_rows = len(df)
    logger.info("  Read %d rows from TileDB", tdb_rows)

    if dry_run:
        logger.info("  [dry-run] Would write %d rows to Iceberg %s", tdb_rows, array_name)
        return MigrationResult(array_name, tdb_rows, 0, True, "dry-run")

    # --- Write to Iceberg ---
    try:
        ice_cls, table_name_override = _get_iceberg_class(array_name)

        # If this array needs a custom table name, create a subclass on the fly
        if table_name_override is not None:
            ice_cls = type(
                f"Iceberg_{array_name}",
                (ice_cls,),
                {"table_name": table_name_override},
            )

        ice_arr = ice_cls(catalog)
        ice_arr.write_df(df, scope=scope, validate=False)
    except Exception as exc:
        return MigrationResult(array_name, tdb_rows, 0, False, f"Iceberg write failed: {exc}")

    # --- Validate row count ---
    try:
        result_df = ice_arr.read_df(
            start=pd.Timestamp("1980-01-01", tz="UTC"),
            end=pd.Timestamp("2100-01-01", tz="UTC"),
            scope=scope,
            validate=False,
        )
        iceberg_rows = len(result_df)
    except Exception as exc:
        return MigrationResult(array_name, tdb_rows, 0, False, f"Iceberg validation read failed: {exc}")

    success = tdb_rows == iceberg_rows
    if not success:
        logger.error(
            "  Row count mismatch: TileDB=%d, Iceberg=%d", tdb_rows, iceberg_rows
        )
    else:
        logger.info("  Row count validated: %d rows", iceberg_rows)

    return MigrationResult(array_name, tdb_rows, iceberg_rows, success)


def migrate_station(
    tdb_root: str,
    network: str,
    station: str,
    campaign: str,
    catalog_mode: str = "glue",
    warehouse: str | None = None,
    arrays: list[str] | None = None,
    dry_run: bool = False,
) -> list[MigrationResult]:
    """
    Migrate all (or selected) TileDB arrays for a station to Iceberg.

    Returns a list of MigrationResult objects, one per array attempted.
    """
    from earthscope_sfg_tools.iceberg_integration.catalog import get_catalog
    from earthscope_sfg_tools.iceberg_integration.scope import Scope

    scope = Scope(network=network, station=station, campaign=campaign)
    catalog = get_catalog(mode=catalog_mode, local_warehouse=warehouse)
    arrays_to_migrate = arrays or _ALL_ARRAYS

    results: list[MigrationResult] = []
    for array_name in arrays_to_migrate:
        if array_name not in _ALL_ARRAYS:
            logger.warning("Unknown array %r, skipping", array_name)
            continue
        result = migrate_array(array_name, tdb_root, catalog, scope, dry_run=dry_run)
        results.append(result)
        print(str(result))

    return results


# ---------------------------------------------------------------------------
# CLI entry point
# ---------------------------------------------------------------------------


def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        description="Migrate TileDB arrays to Apache Iceberg for one station.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument("--tdb-root", required=True, help="Path to station TileDB directory")
    p.add_argument("--network", required=True, help="Network identifier (e.g. SEAFLOOR)")
    p.add_argument("--station", required=True, help="Station identifier (e.g. GCC1)")
    p.add_argument("--campaign", required=True, help="Campaign identifier (e.g. 2024-CRUISE)")
    p.add_argument(
        "--catalog-mode",
        choices=["local", "glue"],
        default="glue",
        help="Iceberg catalog backend (default: glue)",
    )
    p.add_argument(
        "--warehouse",
        default=None,
        help="Warehouse path for local catalog (required when --catalog-mode=local)",
    )
    p.add_argument(
        "--arrays",
        default=None,
        help=f"Comma-separated list of arrays to migrate. Choices: {', '.join(_ALL_ARRAYS)}",
    )
    p.add_argument(
        "--dry-run",
        action="store_true",
        help="Print what would be migrated without writing to Iceberg",
    )
    p.add_argument(
        "--log-level",
        default="INFO",
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        help="Logging verbosity (default: INFO)",
    )
    return p


def main(argv: list[str] | None = None) -> int:
    parser = _build_parser()
    args = parser.parse_args(argv)

    logging.basicConfig(
        level=getattr(logging, args.log_level),
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    if args.catalog_mode == "local" and args.warehouse is None:
        parser.error("--warehouse is required when --catalog-mode=local")

    arrays = [a.strip() for a in args.arrays.split(",")] if args.arrays else None

    results = migrate_station(
        tdb_root=args.tdb_root,
        network=args.network,
        station=args.station,
        campaign=args.campaign,
        catalog_mode=args.catalog_mode,
        warehouse=args.warehouse,
        arrays=arrays,
        dry_run=args.dry_run,
    )

    failed = [r for r in results if not r.success and r.error != "no data"]
    if failed:
        print(f"\n{len(failed)} array(s) failed migration:")
        for r in failed:
            print(f"  {r}")
        return 1

    migrated = [r for r in results if r.success and r.error not in ("dry-run", "no data")]
    print(f"\nMigrated {len(migrated)} array(s) successfully.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
