"""Antenna/radome column regression tests for all metadata entry paths."""

import json

import pytest
from pydantic import ValidationError

from earthscope_sfg_tools.novatel_tools.rinex_metadata import RinexMetadata
from earthscope_sfg_tools.novatel_tools.utils import get_metadatav2


@pytest.mark.parametrize(
    "value,expected",
    [
        ("NOV850 NONE", "NOV850          NONE"),
        ("NOV850          NONE", "NOV850          NONE"),
        ("TRM59800.00 SCIT", "TRM59800.00     SCIT"),
        ("NOV850", "NOV850          NONE"),
        ("ABCDEFGHIJKLMNOP NONE", "ABCDEFGHIJKLMNOPNONE"),
        (None, None),
        ("", ""),
    ],
)
def test_antenna_normalization_round_trip(tmp_path, value, expected):
    metadata = RinexMetadata.load({"marker_name": "SITE", "antenna_model": value})
    assert metadata.antenna_model == expected
    path = metadata.write(tmp_path / "metadata.json")
    assert json.loads(path.read_text())["antenna_model"] == expected
    assert RinexMetadata.load(path).antenna_model == expected


def test_defaults_and_legacy_helper_normalize_antenna():
    assert RinexMetadata(marker_name="SITE").antenna_model == "NOV850          NONE"
    assert get_metadatav2("SITE")["antenna_model"] == "NOV850          NONE"


@pytest.mark.parametrize(
    "value",
    ["A" * 17, "NOV850 TOOLONG", "NOV850 NO!E", "NOV850 NÖNE", "NOV850 NONE EXTRA"],
)
def test_invalid_antenna_metadata_is_rejected(value):
    with pytest.raises(ValidationError):
        RinexMetadata(marker_name="SITE", antenna_model=value)
