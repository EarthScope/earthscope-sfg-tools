# Product Requirements Document

**Feature:** Migrate parsing functionality into `earthscope-sfg-tools`

## Context and Problem Statement

EarthScope is establishing `earthscope-sfg-tools` as the standalone home for seafloor geodesy parsing workflows, but the working parsing functionality still lives in the legacy `earthscope-sfg` package inside the `es_sfgtools` repository. The selected direction is to migrate that functionality into the new package while preserving user-visible behavior and improving architecture, documentation, testing, and dependency boundaries.

## Considered Options

- Keep using the legacy package as the long-term home for parsing.
- Copy legacy code into the new package with minimal structural change.
- Migrate the parsing stack into the new package using layered parser, model, adapter, and integration boundaries.

## Decision Outcome

Proceed with a layered migration into `earthscope-sfg-tools`, preserving behavioral parity for supported workflows while redesigning internal structure for maintainability. Shared validation models remain in scope, while binary-backed and TileDB-backed features should be retained behind clearer optional boundaries where practical.

## Problem Statement

The `earthscope-sfg-tools` package is currently a near-empty standalone package, while the parsing functionality EarthScope relies on still lives inside the legacy `earthscope-sfg` package in the `es_sfgtools` repository. That creates a split-brain development model: the new package is the intended long-term home, but the working parsing logic, data models, binary wrappers, and integration behavior remain coupled to the older codebase and monorepo structure.

This makes it difficult to evolve the new package as a clean, independently released toolkit for seafloor geodesy workflows. Users do not yet have a stable standalone package for parsing NovAtel GNSS logs, Sonardyne SV3 acoustic data, sound velocity profiles, and related derived products. Maintainers also lack a clear architectural target for what should be migrated as-is, what should be redesigned, and what should become optional integrations.

The goal of this effort is to define a product and engineering direction for `earthscope-sfg-tools` that preserves the behavioral value of the legacy parsing stack while improving package architecture, testability, documentation, and dependency boundaries.

## Solution

Develop `earthscope-sfg-tools` as the new standalone Python package for EarthScope seafloor geodesy parsing. The package should migrate the parsing functionality currently provided by the legacy `earthscope-sfg` package, while reorganizing it into layered modules with explicit boundaries between:

- raw input parsers,
- validated domain models,
- dataframe/object adapters,
- optional integration layers for Go binaries and TileDB, and
- compatibility helpers for existing users.

The package should preserve behavioral parity for supported legacy parsing workflows, especially for:

- NovAtel RANGEA parsing and extraction from QC-style inputs,
- Sonardyne SV3 parsing for interrogation/reply workflows and QC JSON ingestion,
- sound speed / CTD profile parsing and interpolation,
- shared Pydantic and Pandera models used to validate parsed outputs,
- RINEX conversion wrappers around existing Go binaries, and
- TileDB-related schema and integration entry points.

The new package should optimize for a layered parser architecture with adapter-style public APIs. That means maintainers should be able to use deeper internal abstractions, while end users should still get straightforward file-to-object or file-to-dataframe helper functions. Validation should continue to use Pydantic and Pandera where they provide clear value, but optional integrations with heavier external dependencies should be isolated behind extras or explicitly optional installation paths to reduce the default dependency footprint.

## User Stories

1. As a seafloor geodesy scientist, I want a standalone package for EarthScope parsing workflows, so that I do not need to depend on the legacy monorepo package structure.
2. As a developer migrating existing notebooks, I want the new package to preserve core parser behavior, so that I can move my imports without silently changing scientific results.
3. As a maintainer, I want a clear separation between raw parsers and downstream transformations, so that I can evolve the codebase without breaking unrelated workflows.
4. As a maintainer, I want parsing code organized into layered modules, so that parser logic, models, adapters, and integrations can be tested independently.
5. As a user working with NovAtel GNSS logs, I want RANGEA messages parsed into structured epoch and observation models, so that I can inspect and process GNSS data programmatically.
6. As a user working with QC PIN-style data, I want raw RANGEA payloads extracted and parsed reliably, so that I can recover GNSS observations from nested JSON sources.
7. As a user working with Sonardyne SV3 observations, I want interrogation and reply events parsed into validated models, so that I can create trustworthy shot-level acoustic datasets.
8. As a user working with DFOP00 event streams, I want file-based helpers that convert those records into validated shot data outputs, so that I can use the data in downstream geodetic workflows.
9. As a user working with QC JSON outputs, I want QC event files converted into the same shot-level structures as DFOP00 inputs, so that equivalent inputs yield equivalent downstream data products.
10. As a user working with sound velocity profiles, I want Seabird and CTD inputs parsed into validated sound speed tables, so that I can use them consistently in processing pipelines.
11. As a scientist, I want interpolation behavior for sound velocity profiles to be explicit and documented, so that I know how missing depth coverage is handled.
12. As a developer, I want shared time, coordinate, and observation models centralized, so that every parser uses the same semantics for timestamps, positions, and validation rules.
13. As a maintainer, I want shared data models to be reusable across parser families, so that schema changes do not require duplicate updates in multiple modules.
14. As a developer, I want a simple high-level API for common file-to-object and file-to-dataframe use cases, so that typical workflows stay approachable.
15. As a power user, I want access to lower-level parser components and normalized models, so that I can build custom pipelines without re-implementing parsing logic.
16. As a user with existing code written against legacy helper functions, I want compatibility adapters where practical, so that migration to the new package is incremental rather than disruptive.
17. As a user who needs RINEX outputs, I want Python wrappers around the existing Go binaries, so that I can invoke those workflows through the new package without dealing with platform-specific binary resolution manually.
18. As a maintainer, I want binary-wrapper functionality clearly separated from pure-Python parsing, so that core parser usage does not require all external tooling.
19. As a user who needs TileDB-backed workflows, I want TileDB schemas and integrations available as optional functionality, so that advanced workflows remain supported without burdening all users.
20. As a release engineer, I want optional dependencies split by feature area, so that the default installation remains lighter while power users can enable advanced integrations explicitly.
21. As a maintainer, I want fixture-based regression tests comparing legacy and new outputs, so that migration preserves externally observable behavior.
22. As a maintainer, I want edge-case tests for malformed inputs, missing blocks, invalid encodings, and partial records, so that the new package fails predictably and safely.
23. As a developer, I want documented examples for the main parsing workflows, so that users can learn the new package quickly.
24. As a maintainer, I want public API boundaries documented, so that future refactors do not accidentally expose unstable internal types.
25. As a scientist, I want parser outputs validated before downstream use, so that bad or incomplete input files are caught early.
26. As a maintainer, I want dependency boundaries defined in the PRD, so that architecture improvements do not accidentally recreate the tight coupling of the legacy package.
27. As a developer, I want the new package to be independently testable in isolation from the rest of the monorepo, so that CI and release workflows are simpler.
28. As a user, I want migration guidance and examples showing old-to-new usage patterns, so that adoption of the new package is straightforward.
29. As a maintainer, I want hidden assumptions in legacy parsing logic surfaced and documented, so that the new implementation is easier to reason about and extend.
30. As a project lead, I want this migration to prioritize parity, architecture, documentation, test coverage, and reduced default installation weight, so that the new package is both trustworthy and maintainable.

## Implementation Decisions

- The new package will become the primary standalone home for EarthScope seafloor geodesy parsing functionality.
- Migration scope includes NovAtel parsing, Sonardyne SV3 parsing, sound speed parsing, shared validation/data models, RINEX Go-binary wrappers, and TileDB integration entry points.
- The architecture will be layered rather than flat. At minimum, the design should distinguish among:
  - raw format parsers,
  - normalized domain models,
  - dataframe/object adapters,
  - integration wrappers for external binaries and storage backends,
  - compatibility-facing helpers for legacy-style usage.
- Public APIs should support both of the following without forcing users into one style:
  - simple high-level helpers for common file-to-output workflows,
  - lower-level parser/model interfaces for advanced usage.
- Pydantic and Pandera will remain part of the design because validation is a core product requirement, not merely an implementation detail.
- To support a lower default dependency footprint, optional or environment-heavy functionality should be isolated by feature area. In particular:
  - pure parser functionality should remain usable without Go binaries,
  - TileDB-related functionality should be installable separately if it introduces additional heavy dependencies,
  - integration code should not leak unnecessary requirements into the parser core.
- Legacy behavior should be treated as the migration baseline for supported workflows. If intentional behavior changes are introduced, they must be explicit, documented, and tested.
- Shared models for time, coordinates, observations, and acoustic records should be centralized to avoid parser-specific schema drift.
- Error handling should be standardized so that malformed or incomplete source data produces predictable exceptions, warnings, or partial-result behavior depending on the workflow contract.
- Compatibility should be defined at the workflow level rather than as a blanket guarantee of one-to-one module parity. The priority is preserving supported user-facing behaviors, not preserving legacy internal structure.
- RINEX conversion and other Go-backed workflows should remain supported through Python wrappers, but their operational requirements should be explicit and isolated from pure parsing.
- TileDB support should remain part of the product direction, but it should be architected as an optional integration surface rather than a mandatory core dependency.
- Documentation should be written alongside the migration, including examples, feature boundaries, installation modes, and migration notes from legacy usage.
- The package should be designed for independent release and testing, without assuming the rest of the legacy monorepo is present.

## Testing Decisions

- A good test validates externally observable behavior, not internal implementation details. For this migration, that means verifying parsed outputs, validated models, dataframe shapes/columns, schema enforcement, and wrapper behavior visible to callers.
- The most important tests are regression tests that compare outputs from the new package to known-good legacy behavior for representative real-world fixtures.
- Test coverage should include at least these areas:
  - NovAtel RANGEA parsing from raw message strings,
  - extraction and parsing of RANGEA content from QC-style JSON inputs,
  - Sonardyne SV3 interrogation and reply parsing,
  - DFOP00 and QC JSON conversion into shot-level outputs,
  - sound speed parsing from Seabird and CTD sources,
  - shared model validation rules,
  - compatibility helpers and high-level public API entry points,
  - optional binary-wrapper workflows and optional TileDB integrations where feasible.
- Tests should cover malformed, partial, and encoding-variant inputs in addition to happy paths.
- Tests for time conversion, coordinate conversion, interpolation, and interrogation/reply merge logic should focus on invariants and user-visible outputs.
- Optional integration tests should be structured so that pure-Python parser tests remain runnable without requiring every external tool.
- Golden fixtures should be introduced for real parser inputs and expected normalized outputs.
- Since no formal test suite was found in the target package and no obvious parser test suite was found in the inspected legacy package, this migration should establish test fixtures and regression baselines as part of the implementation effort rather than assuming prior art already exists.
- Existing examples and known legacy workflows should be mined for fixture candidates, but the new package should define its own durable regression suite.

## Out of Scope

- Reproducing the legacy internal module layout exactly.
- Preserving undocumented implementation quirks unless they are required for supported user-facing behavior.
- Expanding into unrelated workflow areas that are not part of the current parsing and parser-adjacent integration surface.
- Large scientific algorithm changes beyond what is required to migrate, validate, and clarify existing parsing behavior.
- Mandatory installation of all advanced integrations for every user.
- A guarantee that every legacy symbol name will remain unchanged.
- Broad platform/tooling work beyond what is necessary to support the migrated package’s documented installation and runtime paths.

## Further Notes

- The legacy package currently combines pure parsing, downstream transformations, schema validation, and external-tool wrappers. The new package should preserve those capabilities while making the boundaries more explicit.
- A practical migration strategy is to deliver the package in thin vertical slices that establish the shared model layer first, then move parser families one by one, then add compatibility helpers, then finalize optional integrations and documentation.
- The PRD intentionally favors workflow compatibility over source-level copying. The migration should be treated as a chance to improve naming, layering, and dependency isolation without losing trusted behavior.
- Documentation and migration notes are first-class deliverables for this effort, not cleanup work for later.
- If future implementation planning is needed, this PRD should feed directly into a phased implementation plan with clear slices for core models, parser families, compatibility adapters, optional integrations, and documentation/testing milestones.
