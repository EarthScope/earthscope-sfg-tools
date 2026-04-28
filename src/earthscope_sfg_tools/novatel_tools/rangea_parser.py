"""RANGEA ASCII parser and QC extraction helpers."""

from __future__ import annotations

import datetime
import json
from enum import IntEnum
from pathlib import Path

from pydantic import BaseModel, Field, computed_field


class GNSSSystem(IntEnum):
    GPS = 0
    GLONASS = 1
    SBAS = 2
    GALILEO = 3
    BEIDOU = 5
    QZSS = 6
    NAVIC = 7


class Observation(BaseModel):
    signal_type: int
    pseudorange: float
    pseudorange_std: float
    carrier_phase: float
    carrier_phase_std: float
    doppler: float
    cn0: float
    locktime: float
    tracking_status: int
    half_cycle_ambiguity: bool = False
    phase_lock: bool = True
    code_lock: bool = True
    parity_known: bool = True


class Satellite(BaseModel):
    system: GNSSSystem
    prn: int
    fcn: int = 0
    observations: dict[int, Observation] = Field(default_factory=dict)

    def add_observation(self, obs: Observation) -> None:
        self.observations[obs.signal_type] = obs


class GNSSEpoch(BaseModel):
    time: datetime.datetime
    gps_week: int
    gps_seconds: float
    satellites: dict[tuple[int, int], Satellite] = Field(default_factory=dict)
    receiver_status: str = ""
    num_observations: int = 0

    def add_satellite(self, sat: Satellite) -> None:
        key = (int(sat.system), sat.prn)
        if key in self.satellites:
            self.satellites[key].observations.update(sat.observations)
        else:
            self.satellites[key] = sat

    @computed_field
    @property
    def satellite_count(self) -> int:
        return len(self.satellites)


GPS_EPOCH = datetime.datetime(1980, 1, 6, 0, 0, 0, tzinfo=datetime.UTC)
GPS_LEAP_SECONDS = 18


def _decode_channel_tracking_status(status: int) -> dict:
    return {
        "phase_lock_flag": (status >> 10) & 0x07,
        "parity_known": bool((status >> 13) & 0x01),
        "code_lock": bool((status >> 14) & 0x01),
        "system": (status >> 16) & 0x1F,
        "signal_type": (status >> 21) & 0x1F,
        "half_cycle_added": bool((status >> 28) & 0x01),
    }


def _parse_header(header_str: str) -> tuple[int, float, str]:
    fields = header_str.split(",")
    gps_week = None
    gps_seconds = None
    receiver_status = ""

    for i, f in enumerate(fields):
        if f.isdigit() and 2000 <= int(f) <= 3000:
            gps_week = int(f)
            if i + 1 < len(fields):
                try:
                    gps_seconds = float(fields[i + 1])
                    if i + 2 < len(fields):
                        receiver_status = fields[i + 2]
                    break
                except ValueError:
                    continue

    if gps_week is None or gps_seconds is None:
        raise ValueError(f"Could not parse GPS time from header: {header_str}")

    return gps_week, gps_seconds, receiver_status


def _gps_to_utc(gps_week: int, gps_seconds: float, leap_seconds: int = GPS_LEAP_SECONDS):
    total_seconds = gps_week * 604800 + gps_seconds - leap_seconds
    return GPS_EPOCH + datetime.timedelta(milliseconds=total_seconds * 1000)


def deserialize_rangea(rangea_string: str) -> GNSSEpoch:
    if not rangea_string or "#RANGEA" not in rangea_string:
        raise ValueError("Invalid RANGEA string: missing #RANGEA header")

    if "*" in rangea_string:
        rangea_string = rangea_string.split("*")[0]

    parts = rangea_string.split(";")
    if len(parts) != 2:
        raise ValueError("Invalid RANGEA string: missing semicolon separator")

    gps_week, gps_seconds, receiver_status = _parse_header(parts[0])
    epoch = GNSSEpoch(
        time=_gps_to_utc(gps_week, gps_seconds),
        gps_week=gps_week,
        gps_seconds=gps_seconds,
        receiver_status=receiver_status,
    )

    data_fields = parts[1].split(",")
    num_obs = int(data_fields[0])
    epoch.num_observations = num_obs

    idx = 1
    fields_per_obs = 10
    known_system_ids = {e.value for e in GNSSSystem}

    for _ in range(num_obs):
        if idx + fields_per_obs > len(data_fields):
            break

        try:
            prn = int(data_fields[idx])
            glo_freq = int(data_fields[idx + 1])
            psr = float(data_fields[idx + 2])
            psr_std = float(data_fields[idx + 3])
            adr = float(data_fields[idx + 4])
            adr_std = float(data_fields[idx + 5])
            doppler = float(data_fields[idx + 6])
            cn0 = float(data_fields[idx + 7])
            locktime = float(data_fields[idx + 8])
            ch_tr_status = int(data_fields[idx + 9], 16)

            status = _decode_channel_tracking_status(ch_tr_status)
            system_id = status["system"]
            system = GNSSSystem(system_id) if system_id in known_system_ids else GNSSSystem.GPS

            obs = Observation(
                signal_type=status["signal_type"],
                pseudorange=psr,
                pseudorange_std=psr_std,
                carrier_phase=adr,
                carrier_phase_std=adr_std,
                doppler=doppler,
                cn0=cn0,
                locktime=locktime,
                tracking_status=ch_tr_status,
                half_cycle_ambiguity=status["half_cycle_added"],
                phase_lock=status["phase_lock_flag"] >= 3,
                code_lock=status["code_lock"],
                parity_known=status["parity_known"],
            )

            sat_key = (int(system), prn)
            if sat_key not in epoch.satellites:
                fcn = glo_freq if system == GNSSSystem.GLONASS else 0
                epoch.satellites[sat_key] = Satellite(system=system, prn=prn, fcn=fcn)

            epoch.satellites[sat_key].add_observation(obs)

        except (ValueError, IndexError):
            pass

        idx += fields_per_obs

    return epoch


def extract_rangea_strings_from_qcpin(source: str | Path) -> list[str]:
    path = Path(source)
    try:
        with open(path) as f:
            data = json.load(f)
    except UnicodeDecodeError:
        return []

    if not isinstance(data, dict):
        return []

    rangea_strings: list[str] = []

    def _extract_nov_range(obj: dict) -> None:
        if not isinstance(obj, dict):
            return

        if "NOV_RANGE" in obj:
            nov_range = obj["NOV_RANGE"]
            if isinstance(nov_range, dict) and "raw" in nov_range:
                raw_rangea = nov_range["raw"]
                if isinstance(raw_rangea, str) and "#RANGEA" in raw_rangea:
                    rangea_strings.append(raw_rangea)

        if "observations" in obj:
            _extract_nov_range(obj["observations"])

        for value in obj.values():
            if isinstance(value, dict):
                _extract_nov_range(value)

    for value in data.values():
        if isinstance(value, dict):
            _extract_nov_range(value)

    return list(set(rangea_strings))


def extract_rangea_from_qcpin(source: str | Path) -> list[GNSSEpoch]:
    path = Path(source)
    try:
        rangea_strings = extract_rangea_strings_from_qcpin(path)
    except (json.JSONDecodeError, UnicodeDecodeError):
        return []
    return [deserialize_rangea(s) for s in rangea_strings]


def epoch_to_dict(epoch: GNSSEpoch) -> dict:
    return {
        "time": epoch.time.isoformat(),
        "gps_week": epoch.gps_week,
        "gps_seconds": epoch.gps_seconds,
        "receiver_status": epoch.receiver_status,
        "num_observations": epoch.num_observations,
        "satellite_count": epoch.satellite_count,
        "satellites": [
            {
                "system": sat.system.name,
                "prn": sat.prn,
                "fcn": sat.fcn,
                "observations": [
                    {
                        "signal_type": obs.signal_type,
                        "pseudorange": obs.pseudorange,
                        "carrier_phase": obs.carrier_phase,
                        "doppler": obs.doppler,
                        "cn0": obs.cn0,
                        "locktime": obs.locktime,
                    }
                    for obs in sat.observations.values()
                ],
            }
            for sat in epoch.satellites.values()
        ],
    }
