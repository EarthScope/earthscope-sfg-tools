"""Single-class metadata management for RINEX conversion workflows."""

from __future__ import annotations

import json
import uuid
from pathlib import Path

from pydantic import BaseModel, Field, field_validator


class RinexMetadata(BaseModel):
    """Validated, immutable metadata for any NovAtel-to-RINEX conversion.

    Use ``RinexMetadata.load()`` as the single entry point regardless of
    whether the source is a dict, a JSON file path, an existing instance,
    or just a site code.  Call ``write()`` explicitly when a JSON file is
    needed on disk (e.g. to pass to a Go binary via ``-settings``).
    """

    model_config = {"frozen": True}

    marker_name: str = Field(..., description="Site name")
    rinex_version: str | None = Field(default="4.02")
    rinex_type: str | None = Field(default="O")
    rinex_system: str | None = Field(default="M")
    marker_number: str | None = Field(default="0001")
    marker_type: str | None = Field(default="GEODETIC")
    observer: str | None = Field(default="EarthScope")
    agency: str | None = Field(default="EarthScope")
    program: str | None = Field(default="gnsstools")
    run_by: str | None = Field(default="")
    date: str | None = Field(default=None)
    receiver_model: str | None = Field(default="NOV")
    receiver_serial: str | None = Field(default="XXXXXXXXXX")
    receiver_firmware: str | None = Field(default="0.0.0")
    antenna_model: str | None = Field(default="NOV850 NONE", validate_default=True)
    antenna_serial: str | None = Field(default="987654321")
    antenna_position: list[float] | None = Field(
        default_factory=lambda: [0.0, 0.0, 0.0]
    )
    antenna_offsetHEN: list[float] | None = Field(
        default_factory=lambda: [0.0, 0.0, 0.0]
    )

    @field_validator("antenna_model")
    @classmethod
    def normalize_antenna_model(cls, value: str | None) -> str | None:
        """Format the RINEX A20 antenna field as model (16) + radome (4).

        Accept legacy combined strings and already padded fields. A missing
        radome means NONE; reject values that would otherwise be truncated.
        """
        if value is None:
            return None
        fields = value.split()
        if len(value) == 20 and len(fields) == 1:
            fields = [value[:16], value[16:]]
        if not fields:
            return ""
        if len(fields) > 2:
            raise ValueError("antenna_model must contain a model and optional radome")
        model = fields[0]
        radome = fields[1] if len(fields) == 2 else "NONE"
        if not model.isascii() or len(model) > 16:
            raise ValueError("antenna model must be at most 16 ASCII characters")
        if not radome.isascii() or len(radome) != 4 or not radome.isalnum():
            raise ValueError(
                "antenna radome must be four ASCII alphanumeric characters"
            )
        return f"{model:<16}{radome}"

    @classmethod
    def load(
        cls,
        source: "dict | Path | str | RinexMetadata | None" = None,
        *,
        site: str | None = None,
    ) -> "RinexMetadata":
        """Resolve and validate metadata from any supported source.

        Parameters
        ----------
        source:
            dict, Path/str to a JSON file, an existing RinexMetadata (returned
            as-is), or None (requires ``site``).
        site:
            4-character site code.  Used only when ``source`` is None.

        Raises
        ------
        ValueError
            Unsupported ``source`` type, or neither ``source`` nor ``site`` given.
        FileNotFoundError
            ``source`` is a path that does not exist.
        pydantic.ValidationError
            The resolved data fails model validation.
        """
        if source is None:
            if site is None:
                raise ValueError("Either source or site must be provided")
            if len(site) != 4:
                raise ValueError(f"site must be 4 characters, got {site!r}")
            return cls(marker_name=site, receiver_serial=uuid.uuid4().hex[:10])
        if isinstance(source, cls):
            return source
        if isinstance(source, dict):
            return cls(**source)
        if isinstance(source, (str, Path)):
            p = Path(source)
            if not p.exists():
                raise FileNotFoundError(f"Metadata file not found: {p}")
            with p.open() as fh:
                return cls(**json.load(fh))
        raise ValueError(
            f"source must be a dict, path, RinexMetadata, or None; got {type(source)}"
        )

    def write(self, path: Path | str) -> Path:
        """Serialise to JSON and return the resolved path."""
        path = Path(path)
        path.write_text(self.model_dump_json(indent=4))
        return path
