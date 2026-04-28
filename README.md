# earthscope-sfg-tools

Standalone EarthScope seafloor geodesy tooling with a layered architecture for:

- parser/model workflows (NovAtel, Sonardyne, sound speed)
- optional Go-backed high-performance conversions
- optional TileDB integration surfaces

## TileDB integration architecture

TileDB functionality is now exposed through a dedicated integration boundary:

- `earthscope_sfg_tools.tiledb_integration.TileDBService`: high-level facade
- `earthscope_sfg_tools.tiledb_integration.GoBinaryTileDBBackend`: adapter backed by compiled Go binaries
- `earthscope_sfg_tools.tiledb_integration.TileDBOperationResult`: structured operation result
- `earthscope_sfg_tools.tiledb_integration.TileDBBinaryExecutionError`: structured failure mode

This deepens the previous shallow subprocess wrappers into a single service boundary with explicit error policy.

### Migration-safe legacy helpers

Legacy return-code-style calls are supported through:

- `earthscope_sfg_tools.tiledb_integration.nova2tile`
- `earthscope_sfg_tools.tiledb_integration.nov0002tile`
- `earthscope_sfg_tools.tiledb_integration.tdb2rnx`

These compatibility helpers preserve return-code behavior while routing through the new architecture.