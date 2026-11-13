# fileparse file layout: one file per input type, shared files for shared code

## Intent

In `src/go/pkg/fileparse` (one flat `package fileparse`), opening an input
file type's `.go` file should show everything specific to that type. Code
used by more than one file type lives in a few shared files, each named for
the one thing it covers.

## Goals / non-goals

- Goal: one `.go` file per input file type; a small set of shared utility files.
- Non-goal: changing behavior, names or the package API (`Parsers`,
  `ForFileType`, `Parser`, the exported constants and row builders).
- Anti-goal: shared files that mix unrelated formats (rejected approach B).

## Approaches

- **A, picked: one shared file per shared thing:** `gpsa.go`, `novatel.go`,
  `sv3.go`, `rows.go`. Each shared file is named for what it covers.
- B: shared files by layer (`decode.go`, `build.go`). Fewer files, but
  `decode.go` would mix GPSA, NovAtel and SV3 in about 600 lines.
- C: fold in only single-use code and keep the about 12 small shared files.
  Least churn, but keeps the clutter.

## Placement rule

1. A declaration used by one file type only goes in that type's file.
2. Otherwise it goes in the shared file of the format it belongs to:
   `gpsa.go` (GPSA packets), `novatel.go` (NovAtel ASCII, INS and RANGEA
   decoding, and status names), or `sv3.go` (SV3 events and the records
   built from them).
3. Otherwise it goes in `rows.go`: the `Builder` infrastructure, and the
   record types and builders of tables fed by 2+ file types.

## Resulting layout

| File | Contents |
|---|---|
| `fileparse.go` | registry, unchanged |
| `ahrs00.go` | + `xsens.go` (only AHRS00 decodes XSENS); attitude rows stay in `rows.go` (AHRS00 and QCPIN both feed them) |
| `gnss00.go` | + `gnss00_nmea.go`, + the NMEA rows and builders from `rows_nmea.go` (GSV, GSA/DOP, VTG) |
| `tcvr00.go` | + `rows_transceiver.go` |
| `dfop00.go` | + `BuildCommunityStandard` |
| `novatel000.go` | + `BuildINSSTDEV` |
| `ctd.go`, `seabird.go`, `novatel770.go`, `qcpin.go` | unchanged |
| `gpsa.go` | unchanged |
| `novatel.go` | `novatel_ascii.go` + `novatel_ins.go` + `novatel_rangea.go` + `novatel_status.go` |
| `sv3.go` | `sv3.go` + `sv3_rows.go` + `CommunityStandardRow` and `AntennaEpoch` (built by sv3, used by DFOP00's builder) |
| `rows.go` | `rows_doc.go` (as a section comment) + `rows_builder.go` + `rows_records.go` + the TWTT, INS (`INSPVARow`, `INSSTDEVRow`, `BuildINSPVA`), GNSSObs, attitude, antenna and sound speed rows and builders, + `optString` (GNSS00, TCVR00) |

## Calls left to the builder (made by Claude, open to correction)

- `optString` goes in `rows.go`, not a separate `util.go`: one small helper
  doesn't justify a file.
- Record types produced by shared decoders stay with them even when only one
  file type builds the table (`INSSTDEVRow` comes from `novatel.go`, so it
  stays in `rows.go`; only `BuildINSSTDEV` moves to `novatel000.go`).
- Moves keep declaration order and comments, and each folded file's header
  comment becomes a section comment.
- Verify with `go vet` (through the edid overlay while `pkg/catalog/edid.go`
  is still `package edid`); there are no tests in the package.
