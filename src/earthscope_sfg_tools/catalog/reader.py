"""
Read access to the Level 1 Iceberg catalog written by the Go ingest
(``dev/level1``): a local SQLite catalog (``<warehouse>/catalog.db``, catalog
name ``default``) or AWS Glue over an S3 warehouse.

Reads always go through the table's current snapshot. Never read a
warehouse's data folders directly: replacing rows rewrites data files and
leaves the superseded ones on disk until snapshots are expired.
"""

from collections.abc import Iterable
from datetime import datetime, timedelta
from pathlib import Path

import pandas as pd
from pyiceberg.catalog import Catalog, load_catalog
from pyiceberg.expressions import (
    AlwaysTrue,
    And,
    BooleanExpression,
    EqualTo,
    GreaterThanOrEqual,
    In,
    LessThan,
    StartsWith,
)

from ..datamodels.iceberg.schemas import LEVEL1_TABLES, Scope, TableDef
from .timescales import as_utc, gps_minus_utc

# Column each Level 1 table's time window filters on; "time" otherwise.
_TIME_COLUMN = {"travel_times": "T_transmit", "gnss_observations": "time_gpst"}


def open_catalog(
    warehouse: str | Path, *, type: str = "sql", region: str | None = None
) -> Catalog:
    """Open the catalog the Go ingest writes.

    Parameters
    ----------
    warehouse : str or Path
        Local warehouse directory (``type="sql"``) or ``s3://`` URI (``"glue"``).
    type : {"sql", "glue"}
    region : str, optional
        AWS region for Glue; the AWS default chain otherwise.
    """
    if type == "sql":
        wh = Path(warehouse).expanduser().resolve()
        if not (wh / "catalog.db").exists():
            raise FileNotFoundError(f"no catalog.db in {wh}")
        return load_catalog(
            "default", type="sql", uri=f"sqlite:///{wh}/catalog.db", warehouse=f"file://{wh}"
        )
    if type == "glue":
        props = {"type": "glue", "warehouse": str(warehouse)}
        if region:
            props["glue.region"] = region
        return load_catalog("default", **props)
    raise ValueError(f"unknown catalog type {type!r} (want 'sql' or 'glue')")


def stream_edid(scope: Scope, file_type: str, log_source: str) -> str:
    """The stream EDID of a Level 1 stream: network:station:campaign:file_type:log_source."""
    return ":".join((scope.network, scope.station, scope.campaign, file_type, log_source))


class Level1Reader:
    """Scope- and time-windowed reads of the Level 1 tables.

    Parameters
    ----------
    catalog : pyiceberg Catalog
        From :func:`open_catalog`.
    """

    def __init__(self, catalog: Catalog):
        self.catalog = catalog
        self._defs = {d.name: d for d in LEVEL1_TABLES}

    def table_def(self, name: str) -> TableDef:
        try:
            return self._defs[name]
        except KeyError:
            raise KeyError(f"no Level 1 table {name!r}") from None

    def read(
        self,
        table: str,
        scope: Scope,
        start: datetime | str | None = None,
        end: datetime | str | None = None,
        *,
        columns: Iterable[str] | None = None,
        file_type: str | None = None,
        log_source: str | None = None,
        streams: Iterable[str] | None = None,
        where: BooleanExpression | None = None,
    ) -> pd.DataFrame:
        """Rows of a Level 1 table for one scope and UTC window, sorted by time.

        Parameters
        ----------
        table : str
            Level 1 table name, e.g. ``"inspva"``.
        scope : Scope
            Network, station, campaign.
        start, end : datetime or ISO str, optional
            UTC window on the table's time column, start inclusive, end
            exclusive (naive values are UTC).
        columns : iterable of str, optional
            Columns to read; all by default.
        file_type, log_source : str, optional
            Restrict to streams of one source file type (e.g. ``"DFOP00"``)
            and, with both given, one log source.
        streams : iterable of str, optional
            Restrict to these stream EDIDs.
        where : pyiceberg expression, optional
            An extra row filter.
        """
        tdef = self.table_def(table)
        tcol = _TIME_COLUMN.get(table, "time")
        expr = self._scope_filter(tdef, scope)
        if log_source is not None and file_type is None:
            raise ValueError("log_source needs file_type")
        if file_type is not None:
            if log_source is not None:
                expr = And(expr, EqualTo("stream_edid", stream_edid(scope, file_type, log_source)))
            else:
                prefix = ":".join((scope.network, scope.station, scope.campaign, file_type, ""))
                expr = And(expr, StartsWith("stream_edid", prefix))
        if streams is not None:
            expr = And(expr, In("stream_edid", set(streams)))
        for bound, op in ((start, GreaterThanOrEqual), (end, LessThan)):
            if bound is not None:
                t = as_utc(bound)
                if tcol == "time_gpst":  # a GPS wall clock in a UTC-typed column
                    t = t + timedelta(seconds=int(gps_minus_utc(t)[0]))
                expr = And(expr, op(tcol, t.isoformat()))
        if where is not None:
            expr = And(expr, where)
        tbl = self.catalog.load_table(f"{tdef.namespace}.{tdef.name}")
        df = tbl.scan(
            row_filter=expr, selected_fields=tuple(columns) if columns else ("*",)
        ).to_pandas()
        if tcol in df.columns:
            df = df.sort_values(tcol, kind="stable").reset_index(drop=True)
        return df

    @staticmethod
    def _scope_filter(tdef: TableDef, scope: Scope) -> BooleanExpression:
        if tdef.name == "gnss_observations":
            # gnsstools layout: no scope columns; the scope leads stream_edid.
            prefix = ":".join((scope.network, scope.station, scope.campaign, ""))
            return StartsWith("stream_edid", prefix)
        if all(n in (c.name for c in tdef.columns) for n in tdef.scope_names):
            return tdef.scope_filter(scope.network, scope.station, scope.campaign)
        return AlwaysTrue()
