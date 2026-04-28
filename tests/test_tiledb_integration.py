"""Tests for TileDB integration architecture."""

from __future__ import annotations

import unittest
from unittest.mock import patch

from earthscope_sfg_tools.tiledb_integration.errors import TileDBBinaryExecutionError
from earthscope_sfg_tools.tiledb_integration.models import TileDBOperationResult
from earthscope_sfg_tools.tiledb_integration.service import TileDBService


class _FakeBackend:
    def __init__(self, exit_code: int):
        self._exit_code = exit_code

    def write_ascii(self, input_files, tdb_path, num_procs=10, search_paths=None):
        return TileDBOperationResult(
            operation="nova2tile",
            target=tdb_path,
            exit_code=self._exit_code,
            backend="fake",
        )

    def write_binary(self, input_files, tdb_path, search_paths=None):
        return TileDBOperationResult(
            operation="nov0002tile",
            target=tdb_path,
            exit_code=self._exit_code,
            backend="fake",
        )

    def export_rinex(self, tdb_path, settings_file, search_paths=None):
        return TileDBOperationResult(
            operation="tdb2rnx",
            target=tdb_path,
            exit_code=self._exit_code,
            backend="fake",
        )


class TestTileDBService(unittest.TestCase):
    def test_success_result_passes_through(self):
        service = TileDBService(backend=_FakeBackend(exit_code=0))
        result = service.ingest_ascii("input.obs", "s3://array")
        self.assertTrue(result.success)
        self.assertEqual(result.operation, "nova2tile")

    def test_nonzero_exit_raises_by_default(self):
        service = TileDBService(backend=_FakeBackend(exit_code=2))
        with self.assertRaises(TileDBBinaryExecutionError):
            service.export_to_rinex("s3://array", "settings.json")

    def test_nonzero_exit_allowed_when_configured(self):
        service = TileDBService(backend=_FakeBackend(exit_code=3), raise_on_failure=False)
        result = service.ingest_binary("input.nov", "s3://array")
        self.assertFalse(result.success)
        self.assertEqual(result.exit_code, 3)


class TestLegacyCompatibilityWrappers(unittest.TestCase):
    @patch("earthscope_sfg_tools.tiledb_integration.backends.go_binaries.nova2tile")
    def test_legacy_nova2tile_returns_exit_code(self, mock_nova2tile):
        mock_nova2tile.return_value = 7
        from earthscope_sfg_tools.tiledb_integration.legacy import nova2tile

        exit_code = nova2tile("input.obs", "s3://array", num_procs=4)
        self.assertEqual(exit_code, 7)


if __name__ == "__main__":
    unittest.main()
