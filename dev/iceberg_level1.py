"""
Dev harness: parse Level 1 logs with ``sfg level1 parse`` (src/go) and append
them to a local Iceberg warehouse.

Go writes one Arrow IPC file per table holding full rows (the Go file
parsers stamp scope and Log_Source); they are cast to the Iceberg schema,
validated and appended.
"""

import subprocess
import tempfile
from pathlib import Path

import pyarrow as pa
import pyarrow.compute as pc
import pyarrow.ipc as ipc
from pyiceberg.catalog import load_catalog
from pyiceberg.exceptions import NamespaceAlreadyExistsError
from pyiceberg.table import Table

from earthscope_sfg_tools.datamodels.iceberg.schemas import (
    ATTITUDE_AHRS,
    TableDef,
)

GO_DIR = Path(__file__).parents[1] / "src" / "go"

data_path = Path("/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/SFGMain2/cascadia-gorda/SLT1/2026_A_1126/raw")
warehouse_path = Path("/Users/franklyndunbar/Project/SeaFloorGeodesy/Data/ICEBERG")

ahrs = sorted(data_path.glob("*AHRS00.bin"))
nov770 = sorted(data_path.glob("*NOV770.raw"))
novbin = sorted(data_path.glob("*NOV000.bin"))
dfop00 = sorted(data_path.glob("*DFOP00.raw"))


def build_sfg(out: Path) -> Path:
    """Build the Go ``sfg`` binary into ``out`` and return its path."""
    binary = out / "sfg"
    subprocess.run(["go", "build", "-o", str(binary), "./cmd"], cwd=GO_DIR, check=True)
    return binary


def run_level1_parse(binary: Path, file_type: str, source: Path, out: Path) -> dict[str, pa.Table]:
    """Run ``sfg level1 parse`` on one file; return its Arrow tables keyed by table name.

    The scope comes from ``data_path``: .../<network>/<station>/<campaign>/raw.
    """
    network, station, campaign = data_path.parts[-4:-1]
    subprocess.run(
        [str(binary), "level1", "parse", "--type", file_type, "--network", network,
         "--station", station, "--campaign", campaign, "--out", str(out), str(source)],
        check=True,
    )
    return {p.stem: ipc.open_file(p).read_all() for p in out.glob("*.arrow")}


class IcebergCatalog:

    def __init__(self, warehouse_path: Path):
        self.warehouse_path = warehouse_path
        self.catalog = load_catalog(
            "default",
            **{
                'type': 'sql',
                "uri": f"sqlite:///{warehouse_path}/pyiceberg_catalog.db",
                "warehouse": f"file://{warehouse_path}",
            },
        )

    def _table(self, tdef: TableDef) -> Table:
        """Load ``tdef``'s table, creating its namespace and table if needed."""
        try:
            self.catalog.create_namespace(tdef.namespace)
        except NamespaceAlreadyExistsError:
            pass
        return self.catalog.create_table_if_not_exists(
            tdef.identifier,
            schema=tdef.schema,
            partition_spec=tdef.partition_spec,
            sort_order=tdef.sort_order,
        )

    @staticmethod
    def _check_constraints(tdef: TableDef, table: pa.Table) -> None:
        """Raise if any value breaks a column's ge/le/gt/lt/isin constraint."""
        bounds = (("ge", pc.less), ("le", pc.greater), ("gt", pc.less_equal), ("lt", pc.greater_equal))
        for col in tdef.columns:
            values = table[col.name]
            for attr, violates in bounds:
                limit = getattr(col, attr)
                if limit is None:
                    continue
                bad = pc.sum(violates(values, limit)).as_py() or 0
                if bad:
                    raise ValueError(f"{tdef.name}.{col.name}: {bad} rows violate {attr}={limit}")
            if col.isin is not None:
                bad = pc.sum(pc.invert(pc.is_in(values, pa.array(col.isin)))).as_py() or 0
                if bad:
                    raise ValueError(f"{tdef.name}.{col.name}: {bad} rows not in {col.isin}")

    def ingest(self, tdef: TableDef, parsed: pa.Table) -> int:
        """Cast full parsed rows to ``tdef``'s schema, validate, and append.

        Re-ingesting the same file appends duplicate rows.

        Returns
        -------
        int
            Number of rows appended.
        """
        table = parsed.select(tdef.column_names).cast(tdef.schema.as_arrow())
        self._check_constraints(tdef, table)
        if table.num_rows:
            self._table(tdef).append(table)
        return table.num_rows

    def ingest_attitude(self, ahrs_arrow: pa.Table) -> int:
        """Append attitude rows, e.g. ``sfglevel1 ahrs00`` output."""
        return self.ingest(ATTITUDE_AHRS, ahrs_arrow)


if __name__ == "__main__":
    cat = IcebergCatalog(warehouse_path)
    with tempfile.TemporaryDirectory() as tmp:
        binary = build_sfg(Path(tmp))
        for source in ahrs:
            tables = run_level1_parse(binary, "AHRS00", source, Path(tmp) / source.stem)
            n = cat.ingest_attitude(tables["attitude_ahrs"])
            print(f"{source.name}: {n} rows -> {ATTITUDE_AHRS.qualified_name}")
