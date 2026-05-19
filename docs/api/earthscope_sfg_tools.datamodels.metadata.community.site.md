# site

`earthscope_sfg_tools.datamodels.metadata.community.site`

_No docstring._

## class `SFGDTSFSite`

Community-standard site metadata for a GNSS-A seafloor site.

Attributes:
    Site_name: Short human-readable site identifier.
    Campaign: Campaign name this site belongs to.
    TimeOrigin: Reference epoch for the campaign.
    RefFrame: Terrestrial reference frame. Defaults to
        ``'ITRF20'``.
    MTlist: Ordered list of mirror transponder IDs.
    MT_appPos: Approximate ECEF positions keyed by transponder ID.
    ATDoffset: ``[dEast, dNorth, dUp]`` antenna-to-transducer
        offset in metres. Defaults to ``[0, 0, 0]``.

**Fields**

| Name | Type | Description |
|---|---|---|
| `Campaign` | `str` |  |
| `TimeOrigin` | `datetime` |  |
| `RefFrame` | `str` |  |
| `MTlist` | `list` |  |
| `MT_appPos` | `dict` |  |
| `ATDoffset` | `list` |  |
