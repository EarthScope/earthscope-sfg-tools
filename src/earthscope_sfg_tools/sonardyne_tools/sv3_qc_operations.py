from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path

import pandas as pd
from pandera.typing import DataFrame

from ..data_models.observables import ShotDataFrame
from ..data_models.sv3_models import NovatelInterrogationEvent, NovatelRangeEvent
from ..logging import get_logger
from .sv3_operations import (
    merge_interrogation_reply,
    novatel_interrogation_to_garpos_interrogation,
    novatel_reply_to_garpos_reply,
)


def qcjson_to_shotdata(source: str | Path) -> DataFrame[ShotDataFrame] | None:
    path = Path(source)

    try:
        with open(path, encoding="utf-8") as f:
            raw = json.load(f)
    except UnicodeDecodeError:
        get_logger().logwarn(f"UTF-8 decoding failed for {path}, trying latin-1.")
        try:
            with open(path, encoding="latin-1") as f:
                raw = json.load(f)
        except (FileNotFoundError, PermissionError, json.JSONDecodeError) as e:
            get_logger().logerr(f"Error reading QC JSON {path} with latin-1: {e}")
            return None
    except (FileNotFoundError, PermissionError, json.JSONDecodeError) as e:
        get_logger().logerr(f"Error reading QC JSON {path}: {e}")
        return None

    interrogation_raw = raw.get("interrogation")
    if interrogation_raw is None:
        get_logger().logerr(f"QC JSON {path} is missing 'interrogation' block")
        return None

    try:
        interrogation_event = NovatelInterrogationEvent(**interrogation_raw)
        interrogation_parsed = novatel_interrogation_to_garpos_interrogation(interrogation_event)
    except Exception as e:  # noqa: BLE001
        get_logger().logerr(f"Failed to parse interrogation block in {path}: {e}")
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
        get_logger().logerr(f"No valid range entries found in QC JSON {path}")
        return None

    df = pd.DataFrame(processed)
    df["isUpdated"] = False

    return ShotDataFrame.validate(df, lazy=True)


def batch_qc_by_day(
    dataframes: list[pd.DataFrame],
    date_column: str = "pingTime",
) -> dict[str, pd.DataFrame]:
    batched_data = defaultdict(list)

    for df in dataframes:
        if date_column not in df.columns:
            get_logger().logerr(f"DataFrame missing '{date_column}' column.")
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
