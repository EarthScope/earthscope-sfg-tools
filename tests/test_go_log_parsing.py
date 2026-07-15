"""Tests for per-line logrus level forwarding in ``parse_cli_logs``."""

import logging
import subprocess

from earthscope_sfg_tools.utils.go_utils import parse_cli_logs

_MIXED_STDERR = (
    'time="2026-07-15T10:18:29-07:00" level=info msg="array qc_gnss_obs.tdb already exists"\n'
    'time="2026-07-15T10:18:29-07:00" level=warning msg="Error loading .env file"\n'
    'time="2026-07-15T10:18:29-07:00" level=error msg="ingest failed"\n'
)


def _result(stdout="", stderr=""):
    return subprocess.CompletedProcess(args=[], returncode=0, stdout=stdout, stderr=stderr)


def test_each_line_forwarded_at_its_own_level(caplog):
    logger = logging.getLogger("earthscope_sfg_tools.test_go_log_parsing")
    with caplog.at_level(logging.DEBUG, logger=logger.name):
        parse_cli_logs(_result(stderr=_MIXED_STDERR), logger)

    by_level = {r.levelno: r.getMessage() for r in caplog.records}
    assert "already exists" in by_level[logging.INFO]
    # The "error" substring in a warning line must NOT bump it to ERROR.
    assert "Error loading .env" in by_level[logging.WARNING]
    assert "ingest failed" in by_level[logging.ERROR]


def test_info_lines_suppressed_by_setlevel(caplog):
    logger = logging.getLogger("earthscope_sfg_tools.test_go_log_parsing")
    with caplog.at_level(logging.WARNING, logger=logger.name):
        parse_cli_logs(_result(stderr=_MIXED_STDERR), logger)

    messages = [r.getMessage() for r in caplog.records]
    assert not any("already exists" in m for m in messages)  # info dropped
    assert any("ingest failed" in m for m in messages)  # error kept


def test_stderr_lines_without_level_default_to_warning(caplog):
    logger = logging.getLogger("earthscope_sfg_tools.test_go_log_parsing")
    with caplog.at_level(logging.DEBUG, logger=logger.name):
        parse_cli_logs(_result(stderr="unstructured panic trace\n"), logger)

    assert caplog.records
    assert caplog.records[0].levelno == logging.WARNING
