"""
Access API over the GNSS-A Iceberg catalog.

Reads the Level 1 tables the Go ingest writes (``dev/level1``) and exports
them in the formats the processing codes take, starting with GNATSS
posfilter inputs. See plans/level2-posfilter-export.md.
"""

from .reader import Level1Reader, open_catalog, stream_edid

__all__ = ["Level1Reader", "open_catalog", "stream_edid"]
