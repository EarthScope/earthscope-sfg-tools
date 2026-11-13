# fileparse File Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Regroup `src/go/pkg/fileparse` into one `.go` file per input file type plus four shared files (`gpsa.go`, `novatel.go`, `sv3.go`, `rows.go`).

**Architecture:** Only declarations move, between files of the single flat `package fileparse`. Nothing is renamed and nothing changes behaviour. Each move keeps the declaration's doc comment and its order within its source file. A folded file's header comment becomes a section comment (`// --- XSENS MTData2 decoding ---` followed by the old header text) at the point where its code is inserted.

**Tech Stack:** Go 1.2x, gofmt. Imports are fixed by hand: neither goimports nor gopls is usable while the tree doesn't build.

**Spec:** `docs/superpowers/specs/2026-11-13-fileparse-file-layout-design.md`

## Global Constraints

- No identifier is renamed, added or removed. The set of top-level declarations before and after must be identical (see the check in Task 0).
- Shared-file names are exactly `gpsa.go`, `novatel.go`, `sv3.go` and `rows.go`.
- `optString` goes in `rows.go`, not a separate `util.go`.
- `INSPVARow`, `INSSTDEVRow`, `utcMicros` and `BuildINSPVA` stay in `rows.go`. Only `BuildINSSTDEV` moves to `novatel000.go`.
- `AntennaEpoch` and `CommunityStandardRow` go to `sv3.go`. `BuildCommunityStandard` goes to `dfop00.go`.
- Do not commit. The tree has unrelated uncommitted work in progress; leave all changes in the working tree.

## Review Focus

- **Import blocks after a merge:** a merged file may keep an import it no longer uses, or lose one it now needs. `go vet` must report nothing for `./pkg/fileparse/`.
- **Doc comments left behind:** a moved declaration's comment must move with it. Check the diff for a comment block that no longer sits directly above its declaration.
- **Stale file references in comments:** comments that name the old files must point at the new ones, e.g. `fileparse.go`'s package doc, which mentions `rows_*.go`, `novatel_*.go` and `sv3*.go`. Fix them in Task 3.
- **Duplicated package-level comments:** each source file's old header must appear only once, as a section comment.

---

### Task 0: Record the declaration set and set up verification

**Files:**
- Create (scratchpad only): `$S/decls_before.txt`, where `$S` is the session scratchpad.

**Interfaces:**
- Produces: `decls.sh`, which prints the sorted top-level declarations of `pkg/fileparse`, and the vet command `VET`.

- [ ] **Step 1: Write the declaration lister.**
  This is the scratchpad tool `flatten/list.go` (build tag `list`), already present. Run it from the scratchpad flatten directory:
  `go run -tags list list.go <abs path>/src/go/pkg/fileparse . | head -1 | tr ' ' '\n' | sort > $S/decls_before.txt`
  Expected: about 230 lines, including `BuildINSSTDEV`, `optString`, `xsensDecoder` and `sv3Event`.
- [ ] **Step 2: Confirm the baseline vets clean.**
  `VET` = `cd src/go && go vet -overlay $S/ov.json ./pkg/fileparse/`. Expected: no output, exit 0.
  `$S/ov.json` is the existing overlay that maps `pkg/edid/edid.go` to `pkg/catalog/edid.go` and stubs `catalog.Append` / `AppendOption`.

### Task 1: Shared files (`novatel.go`, `sv3.go`, `rows.go`)

**Files:**
- Create: `pkg/fileparse/novatel.go`, made from `novatel_ascii.go`, `novatel_ins.go`, `novatel_rangea.go` and `novatel_status.go`, in that order.
- Modify: `pkg/fileparse/sv3.go`. Append `sv3_rows.go`, then `AntennaEpoch` and `CommunityStandardRow` from `rows_community.go`.
- Create: `pkg/fileparse/rows.go`, in this order:
  1. `rows_doc.go`'s comment, as the file header
  2. `rows_builder.go`
  3. `rows_records.go`
  4. `rows_twtt.go`
  5. `rows_ins.go`, all of it except `BuildINSSTDEV`, which stays put until Task 2
  6. `rows_gnssobs.go`
  7. `rows_attitude.go`
  8. `rows_antenna.go`
  9. `rows_soundspeed.go`
  10. `optString` from `gnss00_nmea.go`
- Delete: `novatel_ascii.go`, `novatel_ins.go`, `novatel_rangea.go`, `novatel_status.go`, `sv3_rows.go`, `rows_doc.go`, `rows_builder.go`, `rows_records.go`, `rows_twtt.go`, `rows_gnssobs.go`, `rows_attitude.go`, `rows_antenna.go`, `rows_soundspeed.go`.
- Keep for now: `rows_ins.go` holding only `BuildINSSTDEV`, `rows_community.go` holding only `BuildCommunityStandard`, and `rows_nmea.go` and `rows_transceiver.go` untouched.

- [ ] **Step 1: Do the moves listed above.**
  Each merged file gets one import block, the union of its sources' imports. Drop any import vet reports as unused.
- [ ] **Step 2: Run `gofmt -l pkg/fileparse`.** Expected: no output.
- [ ] **Step 3: Run `VET`.** Expected: no output.
- [ ] **Step 4: Re-run the lister into `$S/decls_after.txt`, then `diff $S/decls_before.txt $S/decls_after.txt`.** Expected: no output.

### Task 2: Fold single-use code into its file type's file

**Files:**
- `ahrs00.go`: append all of `xsens.go` under a section comment. Delete `xsens.go`.
- `gnss00.go`: append `gnss00_nmea.go` (everything except `optString`, which is already in `rows.go`), then all of `rows_nmea.go`. Delete both.
- `tcvr00.go`: append all of `rows_transceiver.go`. Delete it.
- `dfop00.go`: append `BuildCommunityStandard` from `rows_community.go`. Delete `rows_community.go`.
- `novatel000.go`: append `BuildINSSTDEV` from `rows_ins.go`. Delete `rows_ins.go`.

- [ ] **Step 1: Do the moves listed above, merging imports as in Task 1.**
- [ ] **Step 2: Run `ls pkg/fileparse/*.go`.** Expected exactly:
  `ahrs00.go ctd.go dfop00.go fileparse.go gnss00.go gpsa.go novatel.go novatel000.go novatel770.go qcpin.go rows.go seabird.go sv3.go tcvr00.go`
- [ ] **Step 3: Run `gofmt -l`, `VET` and the declaration diff, as in Task 1 Steps 2–4.** Expected: no output from any of them.

### Task 3: Update comments that name files

**Files:**
- Modify: `pkg/fileparse/fileparse.go`, the package doc paragraph that starts "The rest are shared by the parsers".
- Modify: any comment that `grep -nE "rows_[a-z]+\.go|novatel_[a-z]+\.go|sv3_rows\.go|xsens\.go|gnss00_nmea\.go" pkg/fileparse/*.go pkg/fileparse/*.md` finds.

- [ ] **Step 1: Rewrite the paragraph to name the new shared files.**
  It should read: row records and Arrow builders for tables fed by several file types in `rows.go`; `gpsa.go` (Sonardyne GPSA packets); `novatel.go` (NovAtel logs); `sv3.go` (SV3 events, shared by DFOP00 and QCPIN). Single-use decoders and builders live in their file type's file, e.g. XSENS in `ahrs00.go`.
- [ ] **Step 2: Re-run the grep.** Expected: no output.
- [ ] **Step 3: Run `VET`.** Expected: no output.
