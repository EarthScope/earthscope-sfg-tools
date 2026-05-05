"""Tests for DFOP00 JSONL parsing → GARPOSShotDataFrame."""

import json
import logging
from pathlib import Path

import pandas as pd
import pytest

from earthscope_sfg_tools.sonardyne_tools.sv3_operations import (
    PairResult,
    RejectionReason,
    SV3PairingRules,
    build_shotdata,
    dfop00_to_shotdata,
    novatel_interrogation_to_garpos_interrogation,
    novatel_reply_to_garpos_reply,
    pair_events,
    parse_dfop00_lines,
    parse_jsonl_lines,
)
from earthscope_sfg_tools.sonardyne_tools.sv3_qc_operations import batch_qc_by_day

FIXTURE = Path(__file__).parent / "data" / "test_gcc1_20250822_DFOP00.raw"
log = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="module")
def shotdata():
    df = dfop00_to_shotdata(FIXTURE, log)
    assert df is not None, "dfop00_to_shotdata returned None for the fixture file"
    return df


# ---------------------------------------------------------------------------
# Happy-path parsing
# ---------------------------------------------------------------------------


class TestDfop00Parsing:
    def test_returns_dataframe(self, shotdata):
        """Parser must return a non-None, non-empty DataFrame."""
        assert len(shotdata) > 0

    def test_row_count(self, shotdata):
        """Fixture has 44 interrogation / 135 range events; 40 valid pairs survive
        (IR5210 and IR5211 only appear with range=0 and are rejected)."""
        assert len(shotdata) == 40

    def test_only_ir5209_survives(self, shotdata):
        """IR5210 and IR5211 have range=0 in this fixture and are filtered out."""
        assert sorted(shotdata.transponderID.unique()) == ["IR5209"]

    def test_required_columns_present(self, shotdata):
        required = {
            "transponderID",
            "pingTime",
            "returnTime",
            "tt",
            "dbv",
            "xc",
            "snr",
            "tat",
            "head0",
            "pitch0",
            "roll0",
            "east0",
            "north0",
            "up0",
            "head1",
            "pitch1",
            "roll1",
            "east1",
            "north1",
            "up1",
            "isUpdated",
        }
        assert required.issubset(set(shotdata.columns))

    def test_isupdated_all_false(self, shotdata):
        """isUpdated is set to False on every row by dfop00_to_shotdata."""
        assert (shotdata.isUpdated == False).all()  # noqa: E712

    def test_pingtime_range(self, shotdata):
        """Ping times should span ~11 minutes on 2025-08-22 UTC."""
        assert abs(float(shotdata.pingTime.min()) - 1755820840.0) < 1.0
        assert abs(float(shotdata.pingTime.max()) - 1755821485.0) < 1.0

    def test_travel_time_physical(self, shotdata):
        """One-way travel times should be physically plausible for seafloor geodesy
        (meters to low single-digit seconds at ocean depths)."""
        assert float(shotdata.tt.min()) > 0
        assert float(shotdata.tt.min()) >= 5.37
        assert float(shotdata.tt.max()) <= 5.42

    def test_all_rows_same_day(self, shotdata):
        """All shots in the fixture file are from the same UTC day."""
        batched = batch_qc_by_day([shotdata])
        assert list(batched.keys()) == ["2025-08-22"]
        assert len(batched["2025-08-22"]) == len(shotdata)


# ---------------------------------------------------------------------------
# batch_qc_by_day
# ---------------------------------------------------------------------------


class TestBatchQcByDay:
    def test_single_dataframe_groups_correctly(self, shotdata):
        result = batch_qc_by_day([shotdata])
        assert isinstance(result, dict)
        assert "2025-08-22" in result

    def test_empty_list_returns_empty_dict(self):
        assert batch_qc_by_day([]) == {}

    def test_preserves_row_count(self, shotdata):
        result = batch_qc_by_day([shotdata])
        total = sum(len(v) for v in result.values())
        assert total == len(shotdata)

    def test_date_column_dropped_from_output(self, shotdata):
        result = batch_qc_by_day([shotdata])
        for df in result.values():
            assert "date" not in df.columns


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


class TestDfop00Errors:
    def test_missing_file_returns_none(self, tmp_path):
        result = dfop00_to_shotdata(tmp_path / "ghost.raw", log)
        assert result is None

    def test_empty_file_returns_none(self, tmp_path):
        empty = tmp_path / "empty.raw"
        empty.write_text("")
        result = dfop00_to_shotdata(empty, log)
        assert result is None

    def test_no_valid_pairs_returns_none(self, tmp_path):
        """A file with only interrogation events (no ranges) yields no pairs."""
        only_interrogations = tmp_path / "interrog_only.raw"
        line = json.dumps(
            {
                "event": "interrogation",
                "event_id": 1,
                "observations": {},
                "sequence": 1,
                "time": {"common": 0},
                "type": "SV3",
            }
        )
        only_interrogations.write_text(line + "\n")
        result = dfop00_to_shotdata(only_interrogations, log)
        assert result is None

    def test_corrupt_json_lines_skipped(self, tmp_path):
        """Corrupt JSON lines are skipped; if nothing survives, None is returned."""
        bad = tmp_path / "corrupt.raw"
        bad.write_text("not json\nalso not json\n")
        # Should not raise; corrupt lines are skipped
        try:
            result = dfop00_to_shotdata(bad, log)
        except Exception as exc:
            pytest.fail(f"dfop00_to_shotdata raised on corrupt input: {exc}")


# ---------------------------------------------------------------------------
# Pure pipeline functions
# ---------------------------------------------------------------------------


class TestParseJsonlLines:
    def test_valid_lines_parsed(self):
        """Valid interrogation and range JSON lines produce typed event objects."""
        with open(FIXTURE, encoding="utf-8") as f:
            lines = f.readlines()
        events = parse_jsonl_lines(lines)
        assert len(events) > 0

    def test_corrupt_lines_skipped(self):
        lines = ["not json\n", "{also bad\n", '{"event": "unknown", "x": 1}\n']
        events = parse_jsonl_lines(lines)
        assert events == []

    def test_empty_input_returns_empty(self):
        assert parse_jsonl_lines([]) == []


class TestPairEvents:
    def test_produces_pairs_from_fixture(self):
        """pair_events must yield at least one (interrogation, reply) pair."""
        with open(FIXTURE, encoding="utf-8") as f:
            lines = f.readlines()
        events = parse_jsonl_lines(lines)
        pairs = list(pair_events(events))
        assert len(pairs) > 0

    def test_range_without_preceding_interrogation_skipped(self):
        """Range events that appear before any interrogation must be dropped."""
        with open(FIXTURE, encoding="utf-8") as f:
            lines = f.readlines()
        # Feed only range events (strip interrogation lines)
        range_lines = [l for l in lines if '"range"' in l]
        events = parse_jsonl_lines(range_lines)
        pairs = list(pair_events(events))
        assert pairs == []


class TestBuildShotdata:
    def test_returns_dataframe_from_fixture(self):
        with open(FIXTURE, encoding="utf-8") as f:
            lines = f.readlines()
        events = parse_jsonl_lines(lines)
        pairs = pair_events(events)
        df = build_shotdata(pairs, log)
        assert df is not None
        assert len(df) > 0

    def test_empty_pairs_returns_none(self):
        assert build_shotdata(iter([]), log) is None

    def test_custom_rules_reject_all_short_range(self):
        """Rules with very high min_range_metres should reject everything."""
        with open(FIXTURE, encoding="utf-8") as f:
            lines = f.readlines()
        events = parse_jsonl_lines(lines)
        pairs = pair_events(events)
        strict_rules = SV3PairingRules(min_range_metres=9999.0)
        result = build_shotdata(pairs, log, rules=strict_rules)
        assert result is None


class TestParseDfop00Lines:
    def test_equivalent_to_dfop00_to_shotdata(self):
        """parse_dfop00_lines + in-memory lines must produce the same row count as the
        high-level dfop00_to_shotdata I/O wrapper."""
        with open(FIXTURE, encoding="utf-8") as f:
            lines = f.readlines()
        df_pure = parse_dfop00_lines(lines, log)
        df_io = dfop00_to_shotdata(FIXTURE, log)
        assert df_pure is not None
        assert df_io is not None
        assert len(df_pure) == len(df_io)

    def test_empty_lines_returns_none(self):
        assert parse_dfop00_lines([], log) is None


class TestSV3PairingRules:
    def test_defaults_match_original_constants(self):
        rules = SV3PairingRules()
        assert rules.max_roundtrip_seconds == 15.0
        assert rules.min_range_metres == 1e-3
        assert rules.return_time_tolerance_seconds == 1e-6

    def test_custom_rules_are_frozen(self):
        rules = SV3PairingRules(min_range_metres=0.5)
        with pytest.raises((AttributeError, TypeError)):
            rules.min_range_metres = 1.0  # type: ignore[misc]


class TestPairResult:
    def test_success_result_has_data(self):
        result = PairResult(data={"key": "value"})
        assert result.data is not None
        assert result.rejection is None

    def test_rejection_result_has_reason(self):
        result = PairResult(data=None, rejection=RejectionReason.ZERO_RANGE)
        assert result.data is None
        assert result.rejection == RejectionReason.ZERO_RANGE

    def test_all_rejection_reasons_exist(self):
        reasons = {r for r in RejectionReason}
        assert RejectionReason.ZERO_RANGE in reasons
        assert RejectionReason.ROUNDTRIP_TOO_LONG in reasons
        assert RejectionReason.RETURN_TIME_MISMATCH in reasons
