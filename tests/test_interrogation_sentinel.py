"""Tests for error-sentinel handling in ``parse_qcjson_dict``."""

import logging

from earthscope_sfg_tools.sonardyne_tools.sv3_qc_operations import parse_qcjson_dict

# Interrogation block whose observations are transceiver error sentinels ("ERR3")
# instead of AHRS/GNSS data dicts — validation fails, but this is expected/benign.
_ERR_SENTINEL_RAW = {
    "interrogation": {
        "event": "interrogation",
        "observations": {"AHRS": "ERR3", "GNSS": "ERR3"},
    }
}

# Interrogation block that is malformed in an unexpected way (observations is a
# dict of data dicts, but required fields are missing) — this should stay ERROR.
_UNEXPECTED_BAD_RAW = {
    "interrogation": {
        "event": "interrogation",
        "observations": {"AHRS": {"unexpected": 1}, "GNSS": {"unexpected": 2}},
    }
}


def test_error_sentinel_logged_at_debug(caplog):
    logger = logging.getLogger("earthscope_sfg_tools.test_interrogation_sentinel")
    with caplog.at_level(logging.DEBUG, logger=logger.name):
        result = parse_qcjson_dict(_ERR_SENTINEL_RAW, logger)

    assert result is None
    records = [r for r in caplog.records if "interrogation" in r.getMessage().lower()]
    assert records, "expected a log about the interrogation block"
    assert all(r.levelno == logging.DEBUG for r in records), (
        f"sentinel skip should be DEBUG, got {[r.levelname for r in records]}"
    )


def test_error_sentinel_silent_at_warning(caplog):
    logger = logging.getLogger("earthscope_sfg_tools.test_interrogation_sentinel")
    with caplog.at_level(logging.WARNING, logger=logger.name):
        result = parse_qcjson_dict(_ERR_SENTINEL_RAW, logger)

    assert result is None
    assert not caplog.records  # nothing at WARNING or above


def test_unexpected_parse_failure_still_errors(caplog):
    logger = logging.getLogger("earthscope_sfg_tools.test_interrogation_sentinel")
    with caplog.at_level(logging.DEBUG, logger=logger.name):
        result = parse_qcjson_dict(_UNEXPECTED_BAD_RAW, logger)

    assert result is None
    assert any(r.levelno == logging.ERROR for r in caplog.records), (
        "a genuinely malformed block should still log at ERROR"
    )
