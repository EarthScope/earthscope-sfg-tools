from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path
import logging

import pandas as pd
from pandera.typing import DataFrame

from ..data_models.observables import ShotDataFrame
from ..data_models.sv3_models import NovatelInterrogationEvent, NovatelRangeEvent

from .sv3_operations import (
    merge_interrogation_reply,
    novatel_interrogation_to_garpos_interrogation,
    novatel_reply_to_garpos_reply,
)

logger = logging.getLogger(__name__)

def qcjson_to_shotdata(source: str | Path, logger: logging.Logger) -> DataFrame[ShotDataFrame] | None:
    """Parse a Sonardyne QC JSON file into a validated shot-data DataFrame.

    Reads the QC JSON file (trying UTF-8 first, falling back to latin-1), extracts
    the single ``'interrogation'`` block, then iterates over all ``event = 'range'``
    entries to build matched ping/reply pairs via
    :func:`~sv3_operations.merge_interrogation_reply`.

    Args:
        source: Path to the QC JSON file.
        logger: Logger instance used to report file I/O errors, parse failures,
            and empty-result warnings.

    Returns:
        A validated :class:`ShotDataFrame` with one row per successful ping/reply
        pair, or ``None`` if the file cannot be read, the interrogation block is
        missing, or no valid range entries are found.
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

    interrogation_raw = raw.get("interrogation")
    if interrogation_raw is None:
        logger.error(f"QC JSON {path} is missing 'interrogation' block")
        return None

    try:
        interrogation_event = NovatelInterrogationEvent(**interrogation_raw)
        interrogation_parsed = novatel_interrogation_to_garpos_interrogation(interrogation_event)
    except Exception as e:  # noqa: BLE001
        logger.error(f"Failed to parse interrogation block in {path}: {e}")
        return None

    processed: list[dict] = []

    for key, value in raw.items():
        if key == "interrogation" or not isinstance(value, dict):
            continue
        if value.get("event") != "range":
            continue

        try:
            reply_event = NovatelRangeEvent(**value)
            reply_parsed = novatel_reply_to_garpos_reply(reply_event)
            merged = merge_interrogation_reply(interrogation_parsed, reply_parsed)
        except Exception:  # noqa: BLE001
            continue

        if merged is not None:
            processed.append(merged)

    if not processed:
        logger.error(f"No valid range entries found in QC JSON {path}")
        return None

    df = pd.DataFrame(processed)
    df["isUpdated"] = False

    return ShotDataFrame.validate(df, lazy=True)


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
        df["date"] = pd.to_datetime(df[date_column].apply(lambda x: x * 1e9), utc=True).dt.date

        for date, group in df.groupby("date"):
            batched_data[str(date)].append(group.drop(columns=["date"]))

    for date in batched_data:
        batched_data[date] = pd.concat(batched_data[date], ignore_index=True)

    return dict(batched_data)
