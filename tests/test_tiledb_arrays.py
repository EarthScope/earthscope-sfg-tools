"""Tests for TileDB array write paths and Go-binary wrapper error handling.

Covers two regressions previously worked around downstream in
earthscope-sfg-workflows:
  - TBDArray.write_df must promote a plain 'time' column to the pandas index
    before tiledb.from_pandas (the sparse dimension is mapped from the index),
    and must not crash with NameError when validate=False.
  - The tiledb_integration Go-binary wrappers must raise on non-zero exit
    instead of silently returning a failed CompletedProcess.
"""

import subprocess
from unittest.mock import patch

import pandas as pd
import pytest

tiledb = pytest.importorskip("tiledb")

from earthscope_sfg_tools.tiledb_integration.arrays import TDBKinPositionArray
from earthscope_sfg_tools.tiledb_integration.novatel_tools import go_binaries

GO_BINARIES = "earthscope_sfg_tools.tiledb_integration.novatel_tools.go_binaries"


def _kin_position_df() -> pd.DataFrame:
    """A minimal valid KinPositionDataFrame with 'time' as a plain column."""
    return pd.DataFrame(
        {
            "time": pd.to_datetime(
                ["2025-08-21T00:00:00Z", "2025-08-21T00:00:01Z"]
            ).as_unit("ms"),
            "latitude": [45.0, 45.0001],
            "longitude": [-120.0, -120.0001],
            "height": [10.0, 10.1],
            "east": [0.1, 0.2],
            "north": [0.3, 0.4],
            "up": [0.5, 0.6],
            "number_of_satellites": [8, 9],
            "pdop": [1.2, 1.3],
            "wrms": [0.01, 0.02],
        }
    )


class TestTBDArrayWriteDf:
    @pytest.fixture
    def kin_array(self, tmp_path) -> TDBKinPositionArray:
        return TDBKinPositionArray(tmp_path / "kin_array")

    def test_write_df_round_trips_through_tiledb(self, kin_array):
        """End-to-end: a DataFrame with 'time' as a plain column lands in the
        array (raised before the index fix)."""
        df = _kin_position_df()
        kin_array.write_df(df)

        # array[:] rather than array.df[:] — the latter needs pyarrow,
        # which the tiledb env does not ship.
        with tiledb.open(str(kin_array.uri)) as array:
            written = array[:]
        assert len(written["time"]) == 2
        assert list(written["latitude"]) == [45.0, 45.0001]

    def test_write_df_promotes_time_column_to_index(self, kin_array):
        """'time' arrives as a plain column but the sparse dimension must be
        the index (tiledb.from_pandas maps dimensions from the index)."""
        df = _kin_position_df()
        with patch(
            "earthscope_sfg_tools.tiledb_integration.arrays.tiledb.from_pandas"
        ) as from_pandas:
            kin_array.write_df(df)

        (uri, written), kwargs = from_pandas.call_args
        assert uri == str(kin_array.uri)
        assert kwargs == {"mode": "append"}
        assert written.index.name == "time"
        assert "time" not in written.columns
        assert len(written) == 2

    def test_write_df_without_validation(self, kin_array):
        """validate=False previously crashed with an unbound df_val NameError."""
        df = _kin_position_df()
        with patch(
            "earthscope_sfg_tools.tiledb_integration.arrays.tiledb.from_pandas"
        ) as from_pandas:
            kin_array.write_df(df, validate=False)

        (_, written), _ = from_pandas.call_args
        assert written.index.name == "time"
        assert len(written) == 2


def _completed(returncode: int) -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess(
        args=["sfg"], returncode=returncode, stdout="out", stderr="boom"
    )


WRAPPER_CALLS = [
    ("nova2tile", lambda: go_binaries.nova2tile(["in.txt"], "tdb://array")),
    ("novab2tile", lambda: go_binaries.novb2tile(["in.raw"], "tdb://array")),
    ("nov0002tile", lambda: go_binaries.nov0002tile(["in.bin"], "tdb://array")),
    ("tdb2rnx", lambda: go_binaries.tdb2rnx("tdb://array", "settings.json")),
]


class TestGoBinaryWrappersRaiseOnFailure:
    @pytest.mark.parametrize("subcommand,call", WRAPPER_CALLS)
    def test_nonzero_exit_raises(self, subcommand, call):
        with (
            patch(f"{GO_BINARIES}.find_binary", return_value="/fake/sfg"),
            patch(f"{GO_BINARIES}.subprocess.run", return_value=_completed(1)),
        ):
            with pytest.raises(RuntimeError, match=f"sfg {subcommand} exited 1"):
                call()

    @pytest.mark.parametrize("subcommand,call", WRAPPER_CALLS)
    def test_zero_exit_returns_result(self, subcommand, call):
        with (
            patch(f"{GO_BINARIES}.find_binary", return_value="/fake/sfg"),
            patch(f"{GO_BINARIES}.subprocess.run", return_value=_completed(0)),
        ):
            result = call()
        assert result.returncode == 0
