"""
Access API over the GNSS-A Iceberg catalog.

Reads the Level 0 tables the Go ingest writes (``src/go``) and exports
them in the formats the processing codes take, starting with GNATSS
posfilter inputs. See plans/level1-posfilter-export.md.
"""

from .reader import Level0Reader, open_catalog, stream_edid

__all__ = ["Level0Reader", "open_catalog", "stream_edid"]
