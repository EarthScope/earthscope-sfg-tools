"""Quality-control operations for Sonardyne SV3 logs: per-day batching and shot-data extraction."""

from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path
import logging

import pandas as pd
from pandera.typing import DataFrame

from ..datamodels.observationdata.garpos.observables import GARPOSShotDataFrame
from ..datamodels.observationdata.parsing.sv3_models import (
    NovatelInterrogationEvent,
    NovatelRangeEvent,
)

from .sv3_operations import (
    build_shotdata,
    novatel_interrogation_to_garpos_interrogation,
    novatel_reply_to_garpos_reply,
)

logger = logging.getLogger(__name__)


def parse_qcjson_dict(
    raw: dict,
    logger: logging.Logger,
) -> "DataFrame[GARPOSShotDataFrame] | None":
    """Parse a decoded QC JSON dict into a validated shot-data DataFrame.

    Pure function: no filesystem access.  Pass the result of ``json.load()``.

    Args:
        raw: Python dict decoded from a Sonardyne QC JSON file.
        logger: For missing-block and merge-failure messages.

    Returns:
        Validated GARPOSShotDataFrame, or None if the interrogation block is
        missing or no valid range entries are found.
    """
    interrogation_raw = raw.get("interrogation")
    if interrogation_raw is None:
        logger.error("QC JSON is missing 'interrogation' block")
        return None

    try:
        interrogation_event = NovatelInterrogationEvent(**interrogation_raw)
        interrogation_parsed = novatel_interrogation_to_garpos_interrogation(
            interrogation_event
        )
    except Exception as e:
        logger.error(f"Failed to parse interrogation block: {e}")
        return None

    def _pairs():
        for key, value in raw.items():
            if key == "interrogation" or not isinstance(value, dict):
                continue
            if value.get("event") != "range":
                continue
            try:
                reply_event = NovatelRangeEvent(**value)
                yield interrogation_parsed, novatel_reply_to_garpos_reply(reply_event)
            except Exception:
                continue

    return build_shotdata(_pairs(), logger)


def qcjson_to_shotdata(
    source: str | Path, logger: logging.Logger
) -> "DataFrame[GARPOSShotDataFrame] | None":
    """Parse a Sonardyne QC JSON file into a validated shot-data DataFrame.

    Thin I/O wrapper around :func:`parse_qcjson_dict`.  Tries UTF-8 first,
    falls back to latin-1.

    Args:
        source: Path to the QC JSON file.
        logger: Logger instance for I/O errors and empty-result warnings.

    Returns:
        A validated :class:`GARPOSShotDataFrame`, or ``None`` on read error or
        no valid pairs.
    """
    path = Path(source)

    try:
        with open(path, encoding="utf-8") as f:
            raw = json.load(f)
    except UnicodeDecodeError:
        logger.warning(f"UTF-8 decoding failed for {path}, trying latin-1.")
        try:
            with open(path, encoding="latin-1") as f:
                raw = json.load(f)
        except (FileNotFoundError, PermissionError, json.JSONDecodeError) as e:
            logger.error(f"Error reading QC JSON {path} with latin-1: {e}")
            return None
    except (FileNotFoundError, PermissionError, json.JSONDecodeError) as e:
        logger.error(f"Error reading QC JSON {path}: {e}")
        return None

    return parse_qcjson_dict(raw, logger)


def batch_qc_by_day(
    dataframes: list[pd.DataFrame],
    date_column: str = "pingTime",
) -> dict[str, pd.DataFrame]:
    """Group a list of shot-data DataFrames into per-day batches.

    Converts the ``pingTime`` column (GPS seconds, stored as a float) to a UTC
    date, groups rows by that date across all input DataFrames, and concatenates
    them into a single DataFrame per day.

    Args:
        dataframes: List of shot-data DataFrames, each expected to contain a
            column named ``date_column``.
        date_column: Name of the column holding the ping timestamp in GPS seconds.
            Defaults to ``'pingTime'``.

    Returns:
        A dictionary mapping ISO-format date strings (``'YYYY-MM-DD'``) to
        concatenated DataFrames containing all shots from that day.
    """
    batched_data = defaultdict(list)

    for df in dataframes:
        if date_column not in df.columns:
            logger.error(f"DataFrame missing '{date_column}' column.")
            continue

        if df.empty:
            continue

        df = df.copy()
        df["date"] = pd.to_datetime(
            df[date_column].apply(lambda x: x * 1e9), utc=True
        ).dt.date

        for date, group in df.groupby("date"):
            batched_data[str(date)].append(group.drop(columns=["date"]))

    for date in batched_data:
        batched_data[date] = pd.concat(batched_data[date], ignore_index=True)

    return dict(batched_data)
