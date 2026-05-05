"""Tests for NOV770 dual-antenna RINEX conversion."""

from pathlib import Path
from unittest.mock import patch

import pytest

from earthscope_sfg_tools.novatel_tools import novatel_binary_2rinex
from earthscope_sfg_tools.novatel_tools.utils import MetadataModel
from earthscope_sfg_tools.utils.go_utils import BinaryNotFoundError

FIXTURE_RAW = Path(__file__).parent / "data" / "test_20260331_174553_00030_NOV770.raw"
SITE = "TST1"
WRAP = "earthscope_sfg_tools.novatel_tools.novatel_to_rinex_operations._novatel_2rinex_wrapper"
FIND = "earthscope_sfg_tools.novatel_tools.novatel_to_rinex_operations.find_binary"


@pytest.fixture
def metadata():
    return MetadataModel(marker_name=SITE).model_dump()


@pytest.fixture
def novb2rnxo_available():
    """Skip if the sfg Go binary is not present."""
    from earthscope_sfg_tools.utils.go_utils import find_binary

    try:
        find_binary()
    except BinaryNotFoundError:
        pytest.skip("sfg binary not available")


# ---------------------------------------------------------------------------
# Integration tests — require the real novb2rnxo binary
# ---------------------------------------------------------------------------


class TestNov770DualAntennaIntegration:
    def test_rinex_files_are_nonempty(self, tmp_path, metadata, novb2rnxo_available):
        """Each output RINEX file should exist and have content."""
        rinex_files = novatel_binary_2rinex(
            files=[FIXTURE_RAW],
            writedir=tmp_path,
            metadata=metadata,
        )
        for path in rinex_files:
            assert path.exists()
            assert path.stat().st_size > 0, f"RINEX file {path} is empty"

    def test_rinex_files_written_to_writedir(
        self, tmp_path, metadata, novb2rnxo_available
    ):
        """All output RINEX files should land in writedir."""
        rinex_files = novatel_binary_2rinex(
            files=[FIXTURE_RAW],
            writedir=tmp_path,
            metadata=metadata,
        )
        for path in rinex_files:
            assert path.parent == tmp_path

    def test_site_code_in_rinex_filenames(self, tmp_path, novb2rnxo_available):
        """RINEX filenames should contain the site code."""
        rinex_files = novatel_binary_2rinex(
            files=[FIXTURE_RAW],
            writedir=tmp_path,
            site=SITE,
        )
        for path in rinex_files:
            assert SITE.lower() in path.name.lower()


# ---------------------------------------------------------------------------
# Unit tests — mock the binary layer
# ---------------------------------------------------------------------------


class TestNov770FileRouting:
    def test_raw_file_routed_to_novb2rnxo(self, tmp_path, metadata):
        """A .raw file should invoke the novb2rnx subcommand."""
        with patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []

            novatel_binary_2rinex(
                files=[FIXTURE_RAW], writedir=tmp_path, metadata=metadata
            )

            assert mock_wrap.call_args.kwargs["subcommand"] == "novb2rnx"

    def test_bin_file_routed_to_nov0002rnx(self, tmp_path, metadata):
        """A .bin file should invoke the nov0002rnx subcommand."""
        fake_bin = tmp_path / "fake.bin"
        fake_bin.touch()

        with patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []

            novatel_binary_2rinex(
                files=[fake_bin], writedir=tmp_path, metadata=metadata
            )

            assert mock_wrap.call_args.kwargs["subcommand"] == "nov0002rnx"

    def test_modulo_millis_forwarded_to_wrapper(self, tmp_path, metadata):
        """modulo_millis should be passed through to _novatel_2rinex_wrapper."""
        with patch(FIND), patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []
            novatel_binary_2rinex(
                files=[FIXTURE_RAW],
                writedir=tmp_path,
                metadata=metadata,
                modulo_millis=1000,
            )
        assert mock_wrap.call_args.kwargs["modulo_millis"] == 1000

    def test_metadata_from_site_code(self, tmp_path):
        """Passing a site code should auto-generate metadata with the correct marker_name."""
        with patch(FIND), patch(WRAP) as mock_wrap:
            mock_wrap.return_value = []
            novatel_binary_2rinex(files=[FIXTURE_RAW], writedir=tmp_path, site=SITE)

        assert mock_wrap.call_args.kwargs["metadata"].marker_name == SITE

    def test_missing_file_raises_file_not_found(self, tmp_path, metadata):
        """A .raw path that does not exist should raise FileNotFoundError."""
        missing = tmp_path / "ghost.raw"

        with pytest.raises(FileNotFoundError):
            novatel_binary_2rinex(files=[missing], writedir=tmp_path, metadata=metadata)

    def test_empty_file_list_raises_value_error(self, tmp_path, metadata):
        """An empty file list should raise ValueError."""
        with pytest.raises(ValueError, match="No input files provided"):
            novatel_binary_2rinex(files=[], writedir=tmp_path, metadata=metadata)

    def test_unsupported_extension_raises_value_error(self, tmp_path, metadata):
        """An unsupported file extension should raise ValueError."""
        bad = tmp_path / "data.xyz"
        bad.touch()

        with pytest.raises(ValueError, match="Unsupported file extension"):
            novatel_binary_2rinex(files=[bad], writedir=tmp_path, metadata=metadata)

    def test_no_metadata_and_no_site_raises_value_error(self, tmp_path):
        """Calling without metadata or site should raise ValueError."""
        with pytest.raises(ValueError):
            novatel_binary_2rinex(files=[FIXTURE_RAW], writedir=tmp_path)

    def test_binary_not_found_propagates(self, tmp_path, metadata):
        """BinaryNotFoundError from GoBinaryRunner should propagate to the caller."""
        runner_find = "earthscope_sfg_tools.utils.go_runner.find_binary"
        with patch(runner_find, side_effect=BinaryNotFoundError("not found")):
            with pytest.raises(BinaryNotFoundError):
                novatel_binary_2rinex(
                    files=[FIXTURE_RAW], writedir=tmp_path, metadata=metadata
                )
