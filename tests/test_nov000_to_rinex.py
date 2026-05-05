"""Tests for NOV000 binary → RINEX conversion."""

from pathlib import Path
from unittest.mock import patch

import pytest

from earthscope_sfg_tools.novatel_tools import novatel_binary_2rinex
from earthscope_sfg_tools.novatel_tools.utils import MetadataModel
from earthscope_sfg_tools.utils.go_utils import BinaryNotFoundError

FIXTURE_BIN = (
    Path(__file__).parent / "data" / "test_gcc1_20250821_205746_00062_NOV000.bin"
)
SITE = "GCC1"
WRAP = "earthscope_sfg_tools.novatel_tools.novatel_to_rinex_operations._novatel_2rinex_wrapper"
FIND = "earthscope_sfg_tools.novatel_tools.novatel_to_rinex_operations.find_binary"


@pytest.fixture
def metadata():
    return MetadataModel(marker_name=SITE).model_dump()


@pytest.fixture
def nov0002rnx_available():
    """Skip if the sfg Go binary is not present."""
    from earthscope_sfg_tools.utils.go_utils import find_binary

    try:
        find_binary()
    except BinaryNotFoundError:
        pytest.skip("sfg binary not available")


# ---------------------------------------------------------------------------
# Integration tests — require the real nov0002rnx binary
# ---------------------------------------------------------------------------


class TestNov000ToRinexIntegration:
    def test_produces_rinex_files(self, tmp_path, metadata, nov0002rnx_available):
        """NOV000.bin conversion should produce at least one RINEX file."""
        rinex_files = novatel_binary_2rinex(
            files=[FIXTURE_BIN],
            writedir=tmp_path,
            metadata=metadata,
        )
        assert len(rinex_files) >= 1

    def test_rinex_files_are_nonempty(self, tmp_path, metadata, nov0002rnx_available):
        """Each output RINEX file should exist and have content."""
        rinex_files = novatel_binary_2rinex(
            files=[FIXTURE_BIN],
            writedir=tmp_path,
            metadata=metadata,
        )
        for path in rinex_files:
            assert path.exists()
            assert path.stat().st_size > 0

    def test_rinex_written_to_writedir(self, tmp_path, metadata, nov0002rnx_available):
        """All output files should be in writedir."""
        rinex_files = novatel_binary_2rinex(
            files=[FIXTURE_BIN],
            writedir=tmp_path,
            metadata=metadata,
        )
        for path in rinex_files:
            assert path.parent == tmp_path

    def test_site_code_in_rinex_filenames(self, tmp_path, nov0002rnx_available):
        """RINEX filenames should contain the site code."""
        rinex_files = novatel_binary_2rinex(
            files=[FIXTURE_BIN],
            writedir=tmp_path,
            site=SITE,
        )
        for path in rinex_files:
            assert SITE.lower() in path.name.lower()


# ---------------------------------------------------------------------------
# Unit tests — mock the binary layer
# ---------------------------------------------------------------------------


class TestNov000FileRouting:
    def test_bin_file_routed_to_nov0002rnx(self, tmp_path, metadata):
        """A .bin file must invoke the nov0002rnx subcommand."""
        with patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []

            novatel_binary_2rinex(
                files=[FIXTURE_BIN], writedir=tmp_path, metadata=metadata
            )

            assert mock_wrap.call_args.kwargs["subcommand"] == "nov0002rnx"

    def test_returns_wrapper_output(self, tmp_path, metadata):
        """novatel_binary_2rinex should pass through whatever the wrapper returns."""
        expected = [tmp_path / f"{SITE}_0001.rnx"]
        with patch(FIND), patch(WRAP) as mock_wrap:
            mock_wrap.return_value = expected
            result = novatel_binary_2rinex(
                files=[FIXTURE_BIN], writedir=tmp_path, metadata=metadata
            )
        assert result == expected

    def test_modulo_millis_forwarded(self, tmp_path, metadata):
        """modulo_millis should be passed through to the wrapper."""
        with patch(FIND), patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []
            novatel_binary_2rinex(
                files=[FIXTURE_BIN],
                writedir=tmp_path,
                metadata=metadata,
                modulo_millis=30000,
            )
        assert mock_wrap.call_args.kwargs["modulo_millis"] == 30000

    def test_metadata_from_site_code(self, tmp_path):
        """A site code should auto-generate metadata with the correct marker_name."""
        with patch(FIND), patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []
            novatel_binary_2rinex(files=[FIXTURE_BIN], writedir=tmp_path, site=SITE)

        assert mock_wrap.call_args.kwargs["metadata"].marker_name == SITE

    def test_missing_file_raises(self, tmp_path, metadata):
        """A .bin path that does not exist should raise FileNotFoundError."""
        with pytest.raises(FileNotFoundError):
            novatel_binary_2rinex(
                files=[tmp_path / "ghost.bin"], writedir=tmp_path, metadata=metadata
            )

    def test_binary_not_found_propagates(self, tmp_path, metadata):
        """BinaryNotFoundError from GoBinaryRunner should propagate to the caller."""
        runner_find = "earthscope_sfg_tools.utils.go_runner.find_binary"
        with patch(runner_find, side_effect=BinaryNotFoundError("not found")):
            with pytest.raises(BinaryNotFoundError):
                novatel_binary_2rinex(
                    files=[FIXTURE_BIN], writedir=tmp_path, metadata=metadata
                )

    def test_raw_and_bin_mixed_both_run(self, tmp_path, metadata):
        """Mixing .raw and .bin files should invoke both subcommands."""
        fixture_raw = (
            Path(__file__).parent / "data" / "test_20260331_174553_00030_NOV770.raw"
        )
        with patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []

            novatel_binary_2rinex(
                files=[FIXTURE_BIN, fixture_raw], writedir=tmp_path, metadata=metadata
            )

        assert mock_wrap.call_count == 2
        subcommands = {c.kwargs["subcommand"] for c in mock_wrap.call_args_list}
        assert subcommands == {"nov0002rnx", "novb2rnx"}
