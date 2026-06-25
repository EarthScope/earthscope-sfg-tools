"""
Iceberg array classes for seafloor geodesy data.

Each class mirrors its TileDB counterpart in tiledb_integration/arrays.py and
exposes the same write_df / read_df / get_unique_dates interface. Scope
(network, station, campaign) is a first-class attribute: it is injected as
columns on every write and used as a filter on every read.
"""

import datetime
import logging
from typing import TYPE_CHECKING

import numpy as np
import pandas as pd
import pyarrow as pa
from pyiceberg.catalog import Catalog
from pyiceberg.expressions import And, EqualTo, GreaterThanOrEqual, LessThanOrEqual
from pyiceberg.table import Table

from ..datamodels.observationdata.garpos.observables import (
    AcousticDataFrame,
    GARPOSShotDataFrame,
    IMUPositionDataFrame,
)
from ..datamodels.observationdata.parsing.ppp import KinPositionDataFrame
from ..novatel_tools.rangea_parser import GNSSEpoch
from .catalog import ensure_namespace
from .scope import Scope
from .schemas import (
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

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)

_NAMESPACE = "sfg"


def _to_utc_datetime(
    dt: datetime.datetime | np.datetime64 | datetime.date,
) -> datetime.datetime:
    """Coerce any date-like value to a UTC-aware datetime."""
    if isinstance(dt, np.datetime64):
        dt = pd.Timestamp(dt).to_pydatetime()
    if isinstance(dt, datetime.date) and not isinstance(dt, datetime.datetime):
        dt = datetime.datetime.combine(dt, datetime.time.min)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=datetime.timezone.utc)
    return dt


def _scope_filter(scope: Scope):
    """Build an Iceberg row filter that matches rows for the given scope."""
    return And(
        And(
            EqualTo("network", scope.network),
            EqualTo("station", scope.station),
        ),
        EqualTo("campaign", scope.campaign),
    )


class IcebergArray:
    """
    Base class for Iceberg-backed seafloor geodesy arrays.

    Subclasses must set:
        table_name        str — Iceberg table name within the 'sfg' namespace
        iceberg_schema    pyiceberg.schema.Schema
        partition_spec    pyiceberg.partitioning.PartitionSpec
        dataframe_schema  pandera schema (may be None for GNSS obs)
        time_column       str — primary timestamp column for range queries
    """

    table_name: str = ""
    iceberg_schema = None
    partition_spec = None
    dataframe_schema = None
    time_column: str = "time"

    def __init__(self, catalog: Catalog, location: str | None = None):
        """
        Args:
            catalog:  A PyIceberg Catalog (local SqlCatalog or Glue).
            location: Optional S3 or filesystem URI for the table data.
                      If None the catalog's default warehouse location is used.
        """
        self.catalog = catalog
        self._location = location
        ensure_namespace(catalog, _NAMESPACE)
        self._table: Table = self._load_or_create_table()

    # ------------------------------------------------------------------
    # Table lifecycle
    # ------------------------------------------------------------------

    def _load_or_create_table(self) -> Table:
        identifier = (_NAMESPACE, self.table_name)
        if self.catalog.table_exists(identifier):
            return self.catalog.load_table(identifier)
        kwargs = {}
        if self._location:
            kwargs["location"] = self._location
        return self.catalog.create_table(
            identifier=identifier,
            schema=self.iceberg_schema,
            partition_spec=self.partition_spec,
            **kwargs,
        )

    # ------------------------------------------------------------------
    # Scope helpers
    # ------------------------------------------------------------------

    def _inject_scope(self, df: pd.DataFrame, scope: Scope) -> pd.DataFrame:
        """Add network/station/campaign columns if not already present."""
        df = df.copy()
        for col, val in (
            ("network", scope.network),
            ("station", scope.station),
            ("campaign", scope.campaign),
        ):
            if col not in df.columns:
                df[col] = val
        return df

    # ------------------------------------------------------------------
    # Public interface (mirrors TBDArray)
    # ------------------------------------------------------------------

    def write_df(
        self,
        df: pd.DataFrame,
        scope: Scope | None = None,
        validate: bool = True,
    ) -> None:
        """
        Append a DataFrame to the Iceberg table.

        Args:
            df:       DataFrame to write. Must pass Pandera validation unless
                      validate=False. Scope columns are injected if scope is
                      provided and the columns are not already present.
            scope:    (network, station, campaign) scope. If supplied the three
                      scope columns are added to every row automatically.
            validate: Run Pandera validation before writing. Defaults to True.
        """
        logger.debug("Writing %d rows to %s", len(df), self.table_name)
        if scope is not None:
            df = self._inject_scope(df, scope)
        if validate and self.dataframe_schema is not None:
            df = self.dataframe_schema.validate(df, lazy=True)
        arrow_table = pa.Table.from_pandas(df, schema=self._arrow_schema())
        self._table.append(arrow_table)

    def read_df(
        self,
        start: datetime.datetime | np.datetime64 | datetime.date,
        end: datetime.datetime | np.datetime64 | datetime.date | None = None,
        scope: Scope | None = None,
        validate: bool = True,
        **kwargs,
    ) -> pd.DataFrame:
        """
        Read rows within [start, end], optionally filtered to a specific scope.

        Args:
            start:    Start of the time window (inclusive).
            end:      End of the time window (inclusive). Defaults to +1 day.
            scope:    If provided, only rows matching network/station/campaign
                      are returned.
            validate: Run Pandera validation on the result.

        Returns:
            A pandas DataFrame, or an empty DataFrame when nothing is found.
        """
        start_dt = _to_utc_datetime(start)
        end_dt = _to_utc_datetime(end) if end is not None else start_dt + datetime.timedelta(days=1)

        logger.debug("Reading %s %s→%s", self.table_name, start_dt, end_dt)

        row_filter = And(
            GreaterThanOrEqual(self.time_column, start_dt.isoformat()),
            LessThanOrEqual(self.time_column, end_dt.isoformat()),
        )
        if scope is not None:
            row_filter = And(row_filter, _scope_filter(scope))

        try:
            arrow_table = self._table.scan(row_filter=row_filter).to_arrow()
        except Exception as exc:
            logger.error("Failed to read %s: %s", self.table_name, exc)
            return pd.DataFrame()

        if arrow_table.num_rows == 0:
            logger.warning("No rows found in %s for the given range", self.table_name)
            return pd.DataFrame()

        df = arrow_table.to_pandas()
        if validate and self.dataframe_schema is not None:
            df = self.dataframe_schema.validate(df, lazy=True)
        return df

    def get_unique_dates(
        self,
        field: str | None = None,
        scope: Scope | None = None,
    ) -> np.ndarray:
        """
        Return a sorted array of unique calendar dates present in the table.

        Args:
            field: Column to inspect. Defaults to self.time_column.
            scope: If provided, only rows matching this scope are considered.

        Returns:
            numpy array of datetime64[D], or None on error.
        """
        col = field or self.time_column
        try:
            scan_kwargs = {"selected_fields": (col,)}
            if scope is not None:
                scan_kwargs["row_filter"] = _scope_filter(scope)
            arrow_table = self._table.scan(**scan_kwargs).to_arrow()
            series = arrow_table.column(col).to_pandas()
            dates = pd.to_datetime(series, utc=True).dt.normalize().unique()
            return np.sort(dates.values.astype("datetime64[D]"))
        except Exception as exc:
            logger.error("get_unique_dates failed for %s: %s", self.table_name, exc)
            return None

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _arrow_schema(self) -> pa.Schema | None:
        try:
            from pyiceberg.io.pyarrow import schema_to_pyarrow
            return schema_to_pyarrow(self.iceberg_schema)
        except Exception:
            return None


# ---------------------------------------------------------------------------
# Concrete array classes
# ---------------------------------------------------------------------------


class IcebergKinPositionArray(IcebergArray):
    """Iceberg table for kinematic GNSS position data."""

    table_name = "kin_position"
    iceberg_schema = KinPositionSchema
    partition_spec = KinPositionPartitionSpec
    dataframe_schema = KinPositionDataFrame
    time_column = "time"

    def get_unique_dates(self, field: str = "time", scope: Scope | None = None) -> np.ndarray:
        return super().get_unique_dates(field, scope)


class IcebergIMUPositionArray(IcebergArray):
    """Iceberg table for IMU position and orientation data."""

    table_name = "imu_position"
    iceberg_schema = IMUPositionSchema
    partition_spec = IMUPositionPartitionSpec
    dataframe_schema = IMUPositionDataFrame
    time_column = "time"

    def get_unique_dates(self, field: str = "time", scope: Scope | None = None) -> np.ndarray:
        return super().get_unique_dates(field, scope)


class IcebergAcousticArray(IcebergArray):
    """Iceberg table for acoustic ranging data.

    External interface matches AcousticDataFrame: pingTime and returnTime are
    GPS float seconds. Conversion to/from TimestampTz happens inside this class.
    """

    table_name = "acoustic"
    iceberg_schema = AcousticSchema
    partition_spec = AcousticPartitionSpec
    dataframe_schema = AcousticDataFrame
    time_column = "pingTime"

    def get_unique_dates(self, field: str = "pingTime", scope: Scope | None = None) -> np.ndarray:
        return super().get_unique_dates(field, scope)

    def write_df(
        self,
        df: pd.DataFrame,
        scope: Scope | None = None,
        validate: bool = True,
    ) -> None:
        """Write acoustic data, converting float GPS-second timestamps to datetime."""
        if scope is not None:
            df = self._inject_scope(df, scope)
        if validate and self.dataframe_schema is not None:
            df = self.dataframe_schema.validate(df, lazy=True)

        df = df.copy()
        for col in ("pingTime", "returnTime"):
            if col in df.columns and pd.api.types.is_float_dtype(df[col]):
                df[col] = pd.to_datetime(df[col], unit="s", utc=True)

        arrow_table = pa.Table.from_pandas(df, schema=self._arrow_schema())
        self._table.append(arrow_table)

    def read_df(
        self,
        start: datetime.datetime | np.datetime64 | datetime.date,
        end: datetime.datetime | np.datetime64 | datetime.date | None = None,
        scope: Scope | None = None,
        validate: bool = True,
        **kwargs,
    ) -> pd.DataFrame:
        """Read acoustic data; end defaults to end-of-day. Timestamps returned as floats."""
        start_dt = _to_utc_datetime(start)
        if end is None:
            end_dt = datetime.datetime.combine(
                start_dt.date(), datetime.time.max, tzinfo=datetime.timezone.utc
            )
        else:
            end_dt = _to_utc_datetime(end)

        df = super().read_df(start_dt, end_dt, scope=scope, validate=False, **kwargs)
        if df.empty:
            return df

        for col in ("pingTime", "returnTime"):
            if col in df.columns:
                df[col] = pd.to_datetime(df[col], utc=True).apply(lambda x: x.timestamp())

        if validate and self.dataframe_schema is not None:
            df = self.dataframe_schema.validate(df, lazy=True)
        return df


class IcebergShotDataArray(IcebergArray):
    """Iceberg table for processed shot data."""

    table_name = "shotdata"
    iceberg_schema = ShotDataSchema
    partition_spec = ShotDataPartitionSpec
    dataframe_schema = GARPOSShotDataFrame
    time_column = "pingTime"

    def get_unique_dates(self, field: str = "pingTime", scope: Scope | None = None) -> np.ndarray:
        return super().get_unique_dates(field, scope)

    def write_df(
        self,
        df: pd.DataFrame,
        scope: Scope | None = None,
        validate: bool = True,
    ) -> None:
        """Write shot data, converting float Unix-second timestamps to datetime."""
        if scope is not None:
            df = self._inject_scope(df, scope)
        if validate and self.dataframe_schema is not None:
            df = self.dataframe_schema.validate(df, lazy=True)

        if df.empty:
            logger.warning("Empty DataFrame, skipping write to %s", self.table_name)
            return

        df = df.copy()
        for col in ("pingTime", "returnTime"):
            if col in df.columns:
                if pd.api.types.is_float_dtype(df[col]):
                    df[col] = pd.to_datetime(df[col], unit="s", utc=True)
                else:
                    df[col] = pd.to_datetime(df[col], utc=True)

        arrow_table = pa.Table.from_pandas(df, schema=self._arrow_schema())
        self._table.append(arrow_table)

    def read_df(
        self,
        start: datetime.datetime | np.datetime64 | datetime.date,
        end: datetime.datetime | np.datetime64 | datetime.date | None = None,
        scope: Scope | None = None,
        validate: bool = True,
        **kwargs,
    ) -> pd.DataFrame:
        """Read shot data and convert timestamps back to float Unix seconds."""
        start_dt = _to_utc_datetime(start)
        if end is None:
            end_dt = datetime.datetime.combine(
                start_dt.date(), datetime.time.max, tzinfo=datetime.timezone.utc
            )
        else:
            end_dt = _to_utc_datetime(end)

        df = super().read_df(start_dt, end_dt, scope=scope, validate=False, **kwargs)
        if df.empty:
            return df

        for col in ("pingTime", "returnTime"):
            if col in df.columns:
                df[col] = pd.to_datetime(df[col], utc=True).apply(lambda x: x.timestamp())

        if validate and self.dataframe_schema is not None:
            df = self.dataframe_schema.validate(df, lazy=True)
        return df


class IcebergGNSSObsArray(IcebergArray):
    """
    Iceberg table for GNSS observation data.

    Schema aligns with gnsstools/geodata/gnssiceberg/observations.go so that
    data written by either the Go or Python path lands in the same table.
    """

    table_name = "gnss_obs"
    iceberg_schema = GNSSObsSchema
    partition_spec = GNSSObsPartitionSpec
    dataframe_schema = None
    time_column = "epoch"

    def get_unique_dates(
        self, field: str = "epoch", scope: Scope | None = None
    ) -> np.ndarray:
        return super().get_unique_dates(field, scope)

    def read_df(
        self,
        start: datetime.datetime | np.datetime64 | datetime.date,
        end: datetime.datetime | np.datetime64 | datetime.date | None = None,
        scope: Scope | None = None,
        **kwargs,
    ) -> pd.DataFrame:
        start_dt = _to_utc_datetime(start)
        end_dt = _to_utc_datetime(end) if end is not None else start_dt + datetime.timedelta(days=1)

        row_filter = And(
            GreaterThanOrEqual("epoch", start_dt.isoformat()),
            LessThanOrEqual("epoch", end_dt.isoformat()),
        )
        if scope is not None:
            row_filter = And(row_filter, _scope_filter(scope))

        try:
            arrow_table = self._table.scan(row_filter=row_filter).to_arrow()
        except Exception as exc:
            logger.error("Failed to read gnss_obs: %s", exc)
            return pd.DataFrame()

        return arrow_table.to_pandas()

    def write_epochs(self, epochs: list[GNSSEpoch], scope: Scope | None = None) -> int:
        """
        Write GNSS observation epochs to the Iceberg table.

        The schema matches gnsstools ObservationsIcebergWriter (19 obs fields +
        3 scope fields). constellation and signal are written as human-readable
        strings so the table is queryable by both Python and Go writers.

        Args:
            epochs: List of GNSSEpoch objects to write.
            scope:  Scope to tag rows with. Writes empty strings if None.

        Returns:
            Number of rows written after deduplication.
        """
        network = scope.network if scope else ""
        station = scope.station if scope else ""
        campaign = scope.campaign if scope else ""

        write_ts = pd.Timestamp.now(tz="UTC")

        rows: list[dict] = []
        for epoch in epochs:
            epoch_ts = pd.Timestamp(epoch.time, tz="UTC")
            clock_offset = getattr(epoch, "receiver_clock_offset", None)
            epoch_flag = getattr(epoch, "epoch_flag", None) or None

            for sat in epoch.satellites.values():
                sys_str = sat.system.name  # e.g. "GPS", "GLONASS"
                for obs in sat.observations.values():
                    signal_str = str(obs.signal_type)  # e.g. "C1C"
                    rows.append({
                        "network": network,
                        "station": station,
                        "campaign": campaign,
                        "epoch": epoch_ts,
                        "epoch_pico": 0,
                        "clock_offset": clock_offset,
                        "constellation": sys_str,
                        "satellite": int(sat.prn),
                        "signal": signal_str,
                        "code": obs.pseudorange if obs.pseudorange != 0 else None,
                        "phase": obs.carrier_phase if obs.carrier_phase != 0 else None,
                        "doppler": obs.doppler if obs.doppler != 0 else None,
                        "snr": float(obs.cn0) if obs.cn0 != 0 else None,
                        "loss_of_lock": False,
                        "half_cycle_ambiguity": False,
                        "boc_tracking": False,
                        "lock_time": float(obs.locktime) if obs.locktime else None,
                        "epoch_flag": epoch_flag,
                        "fcn": int(sat.fcn) if sat.fcn else None,
                        "write_time": write_ts,
                    })

        if not rows:
            return 0

        df = (
            pd.DataFrame(rows)
            .drop_duplicates(
                subset=["network", "station", "campaign", "epoch", "constellation", "satellite", "signal"],
                keep="first",
            )
        )
        arrow_table = pa.Table.from_pandas(df, schema=self._arrow_schema())
        self._table.append(arrow_table)
        return len(df)
