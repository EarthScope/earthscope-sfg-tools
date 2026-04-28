# Product Requirements Document

**Feature:** Package Deployment Strategy for `earthscope-sfg-tools`

## Context and Problem Statement

`earthscope-sfg-tools` is being established as a standalone package, but today it has minimal packaging and release scaffolding while important operational capabilities in the legacy ecosystem depend on:

- Go-based utilities (RINEX and TileDB conversion tools),
- CGO compilation against TileDB C libraries,
- platform-specific binary resolution and runtime behavior,
- optional heavy dependencies that should not burden core parsing installs.

Without an explicit deployment strategy, maintainers and users face unclear installation outcomes, brittle cross-platform behavior, and uncertainty about what is guaranteed in a default install versus optional integrations.

## Considered Options

- Keep deployment minimal and defer Go/TileDB integration concerns to downstream users.
- Bundle all Go and TileDB functionality in the default Python package installation.
- Deploy with a layered strategy: stable Python core package, optional integration extras, and clearly versioned/validated Go utility support paths.

## Decision Outcome

Adopt a layered deployment model for `earthscope-sfg-tools`:

- **Core package deployment**: parser-first Python package with no hard dependency on Go or TileDB.
- **Optional integration surfaces**: explicit extras and documented environment setup for TileDB and Go-backed workflows.
- **Release pipeline hardening**: deterministic builds, platform-aware validation, and compatibility checks for binary-backed utilities.

This allows broad adoption for parsing users while preserving advanced utility functionality from the legacy repo as optional, well-governed capabilities.

## Problem Statement

Users need to install and use `earthscope-sfg-tools` reliably across environments while retaining access to legacy capabilities such as TileDB and Go conversion utilities. Today, deployment intent is not clearly defined for:

- what ships in the base package,
- how optional integrations are installed and validated,
- how platform-specific Go binaries are handled,
- what CI/release checks establish support guarantees,
- how version compatibility across Python package, Go binaries, and TileDB libraries is enforced.

This creates risk of environment-specific failures, inconsistent onboarding, and ambiguous support boundaries.

## Solution

Define and implement a deployment PRD that establishes a production-ready packaging and release approach for `earthscope-sfg-tools`, with explicit treatment of legacy TileDB/Go utilities.

The deployment strategy will:

1. Keep a lean, reliable default Python installation for parser workflows.
2. Expose optional integration capabilities via explicit extras and feature flags.
3. Standardize how Go utilities are built, resolved, packaged, and validated.
4. Define TileDB integration policy (required system/runtime dependencies, supported install paths, and failure behavior).
5. Add CI and release criteria that verify core and optional paths before publish.
6. Publish clear install and troubleshooting documentation so users can self-serve setup and diagnose known environment issues.

## User Stories

1. As a Python user, I want `pip install earthscope-sfg-tools` to succeed without Go/TileDB setup, so that I can use parsing functionality immediately.
2. As a scientific user, I want optional TileDB features installable via explicit extras, so that I only install heavyweight dependencies when needed.
3. As an operations engineer, I want explicit support matrices by OS and architecture, so that I know which environments are officially validated.
4. As a maintainer, I want parser-only functionality decoupled from binary toolchains, so that core installs remain reliable.
5. As a maintainer, I want deployment behavior aligned with legacy Go utility expectations, so that migrated workflows keep working.
6. As a developer, I want platform-specific binary resolution rules documented, so that I can predict runtime behavior.
7. As a release manager, I want deterministic build and test pipelines, so that releases are reproducible.
8. As a release manager, I want versioning policy for Python package and utility compatibility, so that users avoid mismatched environments.
9. As a maintainer, I want optional integration smoke tests in CI, so that regressions are detected before release.
10. As a user, I want understandable error messages when optional binaries are missing, so that failures are actionable.
11. As a user, I want clear install docs for TileDB C library prerequisites, so that I can configure my environment correctly.
12. As a user, I want a recommended installation path (pip/conda/pixi), so that setup is straightforward.
13. As a contributor, I want local development tasks for building/testing optional integrations, so that I can verify changes consistently.
14. As a maintainer, I want explicit fallback behavior when Go binaries are unavailable, so that parser workflows still proceed.
15. As a maintainer, I want publish-time checks that validate package metadata and extras, so that install UX remains coherent.
16. As a user, I want release notes to indicate integration changes and compatibility impacts, so that upgrades are safer.
17. As a maintainer, I want deployment guardrails around environment variables and shared library lookup, so that dynamic linking issues are reduced.
18. As a maintainer, I want security-conscious binary handling and provenance in release steps, so that supply-chain risk is reduced.
19. As a user, I want examples showing core-only and integration-enabled workflows, so that I can choose the right setup path.
20. As an organization, I want deployment strategy documented in one place, so that onboarding and support are consistent.

## Implementation Decisions

- Deployment is structured as **core + optional integrations**, not a monolithic all-features install.
- Core package scope includes parsing and standardized model/dataframe outputs.
- Go-backed utilities and TileDB-dependent capabilities are treated as optional integration layers.
- Optional dependencies are exposed through well-named extras and documented environment prerequisites.
- Runtime utility resolution behavior must be deterministic and platform-aware, with clear precedence rules.
- Missing optional binaries/libraries should not break core parser APIs; instead, explicit capability errors should be raised only when optional features are invoked.
- CI must include at least:
  - core parser install/test path,
  - optional TileDB path,
  - optional Go utility path,
  - platform-aware smoke checks for supported OS/arch combinations.
- Release criteria include validation that package metadata, extras, and dependency pins are coherent.
- Documentation must include:
  - installation modes,
  - platform prerequisites,
  - binary and shared-library troubleshooting,
  - migration notes from legacy workflows.
- Compatibility policy must define how Python package releases relate to expected Go utility and TileDB versions.
- Deployment artifacts should prioritize maintainability and reproducibility over maximizing bundled complexity.

## Testing Decisions

- A good deployment test validates externally observable behavior: install success, import success, feature availability, and expected failure modes.
- Core tests should verify parser functionality in a minimal environment without optional integrations.
- Optional integration tests should verify:
  - TileDB-enabled imports and representative I/O paths,
  - Go utility invocation and expected output contract,
  - graceful errors when utilities are absent.
- CI should include smoke tests for binary resolution on each supported platform target.
- Regression tests should cover known dynamic-linking and path-resolution edge cases surfaced by legacy workflows.
- Documentation examples should be executable or verified in CI where feasible.
- Prior art should be taken from legacy Makefile/build behavior and existing utility resolution patterns, then refactored into explicit deployment tests.

## Out of Scope

- Rewriting all legacy Go utilities as pure Python.
- Guaranteeing every historical environment permutation from legacy workflows.
- Broad infrastructure redesign unrelated to package deployment and release behavior.
- Shipping all optional system libraries as mandatory components in the default install.
- Perfect backward compatibility for undocumented legacy deployment behaviors.

## Further Notes

- Legacy Go + TileDB utilities are strategically important and should remain supported, but only through clearly bounded optional integration paths.
- The package should communicate capability tiers explicitly (core vs integration-enabled) to reduce user confusion.
- Migration success depends as much on deployment clarity as on parser parity.
- This PRD should feed directly into an implementation plan covering packaging metadata, extras, CI matrix, release workflow, and docs updates.
