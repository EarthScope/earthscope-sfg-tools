# metadata

`earthscope_sfg_tools.rinex_tools.metadata`

Metadata helpers for RINEX workflows.

Core metadata model + validation live in ``earthscope_sfg_tools.novatel_tools.utils``
and are re-exported here to preserve historical import paths.

## `get_metadata(site: str, serialNumber: str = 'XXXXXXXXXX') -> dict`

Return a placeholder RINEX metadata dict for a given site.

.. note::
    Fields are hard-coded placeholders.  Replace with real site
    metadata before using in production conversions.

Args:
    site: 4-character site/marker name.
    serialNumber: Receiver serial number. Defaults to
        ``'XXXXXXXXXX'``.

Returns:
    Dict with ``markerName``, ``markerType``, ``observer``,
    ``agency``, ``receiver``, and ``antenna`` keys.
