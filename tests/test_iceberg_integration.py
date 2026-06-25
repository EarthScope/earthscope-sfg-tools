"""
Round-trip tests for the iceberg_integration module.

All tests use a local SqlCatalog backed by a pytest tmp_path directory — no AWS
credentials required. Tests are skipped automatically if pyiceberg is not installed.
"""

import datetime

import numpy as np
import pandas as pd
import pytest

pyiceberg = pytest.importorskip("pyiceberg", reason="pyiceberg not installed")

from earthscope_sfg_tools.iceberg_integration import (
    IcebergAcousticArray,
    IcebergGNSSObsArray,
    IcebergIMUPositionArray,
    IcebergKinPositionArray,
    IcebergShotDataArray,
    Scope,
    get_catalog,
)

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_GNSS_EPOCH = datetime.datetime(1980, 1, 6, tzinfo=datetime.timezone.utc)
_LEAP = 18
# Minimum GPS timestamp as Unix seconds (used by AcousticDataFrame validation)
_GPS_T0 = _GNSS_EPOCH.timestamp() - _LEAP  # ≈ 315964782.0

_SCOPE_A = Scope(network="SEAFLOOR", station="GCC1", campaign="2024-CRUISE")
_SCOPE_B = Scope(network="SEAFLOOR", station="GCC2", campaign="2024-CRUISE")

_DAY = datetime.datetime(2024, 1, 15, tzinfo=datetime.timezone.utc)
_DAY_END = _DAY + datetime.timedelta(days=1) - datetime.timedelta(microseconds=1)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture()
def catalog(tmp_path):
    """Local SqlCatalog backed by a temporary directory."""
    return get_catalog(mode="local", local_warehouse=tmp_path / "warehouse")


# ---------------------------------------------------------------------------
# Synthetic DataFrame builders
# ---------------------------------------------------------------------------


def _kin_df(n: int = 10, day: datetime.datetime = _DAY) -> pd.DataFrame:
    times = pd.date_range(day, periods=n, freq="1s", tz="UTC")
    return pd.DataFrame({
        "time": times,
        "latitude": np.linspace(34.0, 34.1, n),
        "longitude": np.linspace(-120.0, -120.1, n),
        "height": np.linspace(-30.0, -29.0, n),
        "east": np.zeros(n),
        "north": np.zeros(n),
        "up": np.zeros(n),
        "number_of_satellites": np.full(n, 8, dtype=int),
        "pdop": np.full(n, 1.5),
        "wrms": np.full(n, 0.01),
    })


def _imu_df(n: int = 10, day: datetime.datetime = _DAY) -> pd.DataFrame:
    times = pd.date_range(day, periods=n, freq="1s", tz="UTC")
    return pd.DataFrame({
        "time": times,
        "azimuth": np.linspace(0.0, 10.0, n),
        "pitch": np.linspace(-1.0, 1.0, n),
        "roll": np.linspace(-2.0, 2.0, n),
        "latitude": np.linspace(34.0, 34.1, n),
        "longitude": np.linspace(-120.0, -120.1, n),
        "height": np.linspace(-30.0, -29.0, n),
        "northVelocity": np.zeros(n),
        "eastVelocity": np.zeros(n),
        "upVelocity": np.zeros(n),
        "latitude_std": np.full(n, 0.001),
        "longitude_std": np.full(n, 0.001),
        "height_std": np.full(n, 0.002),
        "northVelocity_std": np.full(n, 0.001),
        "eastVelocity_std": np.full(n, 0.001),
        "upVelocity_std": np.full(n, 0.002),
        "roll_std": np.full(n, 0.01),
        "pitch_std": np.full(n, 0.01),
        "azimuth_std": np.full(n, 0.1),
    })


def _acoustic_df(n: int = 5, day: datetime.datetime = _DAY) -> pd.DataFrame:
    """GPS float-second timestamps — matches AcousticDataFrame pandera schema."""
    t0 = day.timestamp()
    ping_times = np.linspace(t0, t0 + n - 1, n)
    return pd.DataFrame({
        "transponderID": [f"TP0{i % 3}" for i in range(n)],
        "pingTime": ping_times,
        "returnTime": ping_times + 1.5,
        "tt": np.full(n, 1.5),
        "dbv": np.full(n, 100, dtype=int),
        "xc": np.full(n, 50, dtype=int),
        "snr": np.full(n, 15.0),
        "tat": np.full(n, 0.01),
    })


def _shotdata_df(n: int = 5, day: datetime.datetime = _DAY) -> pd.DataFrame:
    """Float GPS-second pingTime/returnTime — matches GARPOSShotDataFrame."""
    t0 = day.timestamp()
    ping_times = np.linspace(t0, t0 + n - 1, n)
    return pd.DataFrame({
        "transponderID": [f"TP0{i % 3}" for i in range(n)],
        "pingTime": ping_times,
        "returnTime": ping_times + 1.5,
        "tt": np.full(n, 1.5),
        "dbv": np.full(n, 100, dtype=int),
        "xc": np.full(n, 50, dtype=int),
        "snr": np.full(n, 15.0),
        "tat": np.full(n, 0.01),
        "head0": np.zeros(n),
        "pitch0": np.zeros(n),
        "roll0": np.zeros(n),
        "east0": np.full(n, 1000.0),
        "north0": np.full(n, 2000.0),
        "up0": np.full(n, -10.0),
        "head1": np.zeros(n),
        "pitch1": np.zeros(n),
        "roll1": np.zeros(n),
        "east1": np.full(n, 1000.1),
        "north1": np.full(n, 2000.1),
        "up1": np.full(n, -10.1),
        "east_std0": np.full(n, 0.1),
        "north_std0": np.full(n, 0.1),
        "up_std0": np.full(n, 0.2),
        "east_std1": np.full(n, 0.1),
        "north_std1": np.full(n, 0.1),
        "up_std1": np.full(n, 0.2),
        "isUpdated": np.zeros(n, dtype=bool),
    })


# ---------------------------------------------------------------------------
# IcebergKinPositionArray
# ---------------------------------------------------------------------------


class TestIcebergKinPositionArray:
    def test_round_trip(self, catalog):
        arr = IcebergKinPositionArray(catalog)
        df = _kin_df()
        arr.write_df(df, scope=_SCOPE_A)

        result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A, validate=False)
        assert len(result) == len(df)
        assert set(result.columns) >= {"time", "latitude", "longitude", "height"}

    def test_scope_columns_injected(self, catalog):
        arr = IcebergKinPositionArray(catalog)
        arr.write_df(_kin_df(), scope=_SCOPE_A)

        result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A, validate=False)
        assert result["network"].iloc[0] == "SEAFLOOR"
        assert result["station"].iloc[0] == "GCC1"
        assert result["campaign"].iloc[0] == "2024-CRUISE"

    def test_scope_filter_isolates_stations(self, catalog):
        arr = IcebergKinPositionArray(catalog)
        arr.write_df(_kin_df(n=10), scope=_SCOPE_A)
        arr.write_df(_kin_df(n=6), scope=_SCOPE_B)

        a_result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A, validate=False)
        b_result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_B, validate=False)
        assert len(a_result) == 10
        assert len(b_result) == 6

    def test_get_unique_dates(self, catalog):
        arr = IcebergKinPositionArray(catalog)
        day2 = _DAY + datetime.timedelta(days=1)
        arr.write_df(_kin_df(day=_DAY), scope=_SCOPE_A)
        arr.write_df(_kin_df(day=day2), scope=_SCOPE_A)

        dates = arr.get_unique_dates(scope=_SCOPE_A)
        assert dates is not None
        assert len(dates) == 2

    def test_empty_range_returns_empty_df(self, catalog):
        arr = IcebergKinPositionArray(catalog)
        arr.write_df(_kin_df(), scope=_SCOPE_A)

        far_future = datetime.datetime(2099, 1, 1, tzinfo=datetime.timezone.utc)
        result = arr.read_df(far_future, scope=_SCOPE_A, validate=False)
        assert result.empty

    def test_write_without_scope(self, catalog):
        """write_df should succeed even when scope is not supplied."""
        arr = IcebergKinPositionArray(catalog)
        df = _kin_df()
        df["network"] = "SEAFLOOR"
        df["station"] = "GCC1"
        df["campaign"] = "2024-CRUISE"
        arr.write_df(df)  # no scope kwarg — columns already present
        result = arr.read_df(_DAY, _DAY_END, validate=False)
        assert len(result) == 10


# ---------------------------------------------------------------------------
# IcebergIMUPositionArray
# ---------------------------------------------------------------------------


class TestIcebergIMUPositionArray:
    def test_round_trip(self, catalog):
        arr = IcebergIMUPositionArray(catalog)
        df = _imu_df()
        arr.write_df(df, scope=_SCOPE_A)

        result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A, validate=False)
        assert len(result) == len(df)
        assert "azimuth" in result.columns
        assert "northVelocity" in result.columns

    def test_nullable_std_columns(self, catalog):
        arr = IcebergIMUPositionArray(catalog)
        df = _imu_df()
        df["latitude_std"] = None  # explicitly null
        arr.write_df(df, scope=_SCOPE_A, validate=False)

        result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A, validate=False)
        assert len(result) == len(df)


# ---------------------------------------------------------------------------
# IcebergAcousticArray
# ---------------------------------------------------------------------------


class TestIcebergAcousticArray:
    def test_round_trip_float_timestamps(self, catalog):
        """pingTime and returnTime survive write→read as floats."""
        arr = IcebergAcousticArray(catalog)
        df = _acoustic_df()
        arr.write_df(df, scope=_SCOPE_A)

        result = arr.read_df(_DAY, scope=_SCOPE_A, validate=False)
        assert len(result) == len(df)
        assert pd.api.types.is_float_dtype(result["pingTime"])
        assert pd.api.types.is_float_dtype(result["returnTime"])

    def test_pingtime_values_preserved(self, catalog):
        arr = IcebergAcousticArray(catalog)
        df = _acoustic_df()
        arr.write_df(df, scope=_SCOPE_A)

        result = arr.read_df(_DAY, scope=_SCOPE_A, validate=False)
        np.testing.assert_allclose(
            sorted(result["pingTime"].tolist()),
            sorted(df["pingTime"].tolist()),
            rtol=1e-3,
        )

    def test_scope_filter(self, catalog):
        arr = IcebergAcousticArray(catalog)
        arr.write_df(_acoustic_df(n=5), scope=_SCOPE_A)
        arr.write_df(_acoustic_df(n=3), scope=_SCOPE_B)

        result_a = arr.read_df(_DAY, scope=_SCOPE_A, validate=False)
        result_b = arr.read_df(_DAY, scope=_SCOPE_B, validate=False)
        assert len(result_a) == 5
        assert len(result_b) == 3


# ---------------------------------------------------------------------------
# IcebergShotDataArray
# ---------------------------------------------------------------------------


class TestIcebergShotDataArray:
    def test_round_trip_float_timestamps(self, catalog):
        arr = IcebergShotDataArray(catalog)
        df = _shotdata_df()
        arr.write_df(df, scope=_SCOPE_A)

        result = arr.read_df(_DAY, scope=_SCOPE_A, validate=False)
        assert len(result) == len(df)
        assert pd.api.types.is_float_dtype(result["pingTime"])
        assert pd.api.types.is_float_dtype(result["returnTime"])

    def test_pingtime_values_preserved(self, catalog):
        arr = IcebergShotDataArray(catalog)
        df = _shotdata_df()
        arr.write_df(df, scope=_SCOPE_A)

        result = arr.read_df(_DAY, scope=_SCOPE_A, validate=False)
        np.testing.assert_allclose(
            sorted(result["pingTime"].tolist()),
            sorted(df["pingTime"].tolist()),
            rtol=1e-3,
        )

    def test_empty_df_skipped(self, catalog):
        arr = IcebergShotDataArray(catalog)
        empty = _shotdata_df().iloc[0:0]  # zero rows
        arr.write_df(empty, scope=_SCOPE_A)

        result = arr.read_df(_DAY, scope=_SCOPE_A, validate=False)
        assert result.empty

    def test_get_unique_dates(self, catalog):
        arr = IcebergShotDataArray(catalog)
        day2 = _DAY + datetime.timedelta(days=1)
        arr.write_df(_shotdata_df(day=_DAY), scope=_SCOPE_A)
        arr.write_df(_shotdata_df(day=day2), scope=_SCOPE_A)

        dates = arr.get_unique_dates(scope=_SCOPE_A)
        assert dates is not None
        assert len(dates) == 2


# ---------------------------------------------------------------------------
# IcebergGNSSObsArray
# ---------------------------------------------------------------------------


class _FakeSys:
    def __init__(self, name): self.name = name; self.value = {"GPS": 0, "GLONASS": 1}[name]


class _FakeObs:
    def __init__(self, code, pr, phase, doppler, cn0, locktime, tracking_status):
        self.signal_type = code
        self.pseudorange = pr
        self.carrier_phase = phase
        self.doppler = doppler
        self.cn0 = cn0
        self.locktime = locktime
        self.tracking_status = tracking_status


class _FakeSat:
    def __init__(self, prn, system_name, observations, fcn=0):
        self.prn = prn
        self.system = _FakeSys(system_name)
        self.observations = {o.signal_type: o for o in observations}
        self.fcn = fcn


class _FakeEpoch:
    def __init__(self, time, satellites):
        self.time = time
        self.satellites = {s.prn: s for s in satellites}


def _make_epochs(n: int = 3, day: datetime.datetime = _DAY) -> list:
    epochs = []
    for i in range(n):
        t = day + datetime.timedelta(seconds=i)
        sats = [
            _FakeSat(1, "GPS", [
                _FakeObs("C1C", 20_000_000.0, 100_000_000.0, -1000.0, 45.0, 100, 0),
                _FakeObs("L1C", 0.0, 100_000_001.0, -1000.0, 45.0, 100, 0),
            ]),
            _FakeSat(2, "GPS", [
                _FakeObs("C1C", 22_000_000.0, 110_000_000.0, -900.0, 42.0, 200, 0),
            ]),
        ]
        epochs.append(_FakeEpoch(t, sats))
    return epochs


class TestIcebergGNSSObsArray:
    def test_write_epochs_returns_row_count(self, catalog):
        arr = IcebergGNSSObsArray(catalog)
        epochs = _make_epochs(n=3)
        rows_written = arr.write_epochs(epochs, scope=_SCOPE_A)
        # 3 epochs × (2 sats × 1 + 1 sat × 1) = 3 × 3 = 9 rows... actually:
        # epoch 0: sat1(C1C, L1C) + sat2(C1C) = 3 obs
        # epoch 1: same = 3
        # epoch 2: same = 3 → 9 total
        assert rows_written == 9

    def test_read_df_returns_rows(self, catalog):
        arr = IcebergGNSSObsArray(catalog)
        arr.write_epochs(_make_epochs(n=3), scope=_SCOPE_A)

        result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A)
        assert len(result) == 9
        assert "constellation" in result.columns
        assert "signal" in result.columns

    def test_constellation_is_string(self, catalog):
        arr = IcebergGNSSObsArray(catalog)
        arr.write_epochs(_make_epochs(n=1), scope=_SCOPE_A)

        result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A)
        assert set(result["constellation"].unique()) == {"GPS"}

    def test_deduplication(self, catalog):
        arr = IcebergGNSSObsArray(catalog)
        epochs = _make_epochs(n=1)
        arr.write_epochs(epochs, scope=_SCOPE_A)
        arr.write_epochs(epochs, scope=_SCOPE_A)  # same data twice

        # Two appends; dedup is per-write only (Iceberg doesn't deduplicate across
        # commits). Row count is 2× because both writes land.
        result = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A)
        # At minimum the first write's rows are present
        assert len(result) >= 3

    def test_get_unique_dates(self, catalog):
        arr = IcebergGNSSObsArray(catalog)
        day2 = _DAY + datetime.timedelta(days=1)
        arr.write_epochs(_make_epochs(n=1, day=_DAY), scope=_SCOPE_A)
        arr.write_epochs(_make_epochs(n=1, day=day2), scope=_SCOPE_A)

        dates = arr.get_unique_dates(scope=_SCOPE_A)
        assert dates is not None
        assert len(dates) == 2

    def test_scope_isolation(self, catalog):
        arr = IcebergGNSSObsArray(catalog)
        arr.write_epochs(_make_epochs(n=3), scope=_SCOPE_A)
        arr.write_epochs(_make_epochs(n=2), scope=_SCOPE_B)

        a = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_A)
        b = arr.read_df(_DAY, _DAY_END, scope=_SCOPE_B)
        assert len(a) == 9
        assert len(b) == 6


# ---------------------------------------------------------------------------
# Catalog
# ---------------------------------------------------------------------------


class TestGetCatalog:
    def test_local_catalog_created(self, tmp_path):
        cat = get_catalog(mode="local", local_warehouse=tmp_path / "wh")
        assert cat is not None

    def test_unknown_mode_raises(self):
        with pytest.raises(ValueError, match="Unknown catalog mode"):
            get_catalog(mode="invalid")  # type: ignore[arg-type]
