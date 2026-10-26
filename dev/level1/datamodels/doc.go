// Package datamodels is the data model of the Level 1 tables, shared by the
// file parsers, the catalog and the sinks:
//
//   - Table definitions (TableDef): each table's name, columns, constraints
//     and layout, e.g. INSPVA, TWTT, AttitudeAHRS, GNSSObs. They mirror the
//     Python earthscope_sfg_tools.datamodels.iceberg.schemas TableDefs.
//   - Row records and builders, one pair per table: a Go record type (XRow,
//     e.g. INSPVARow) and BuildX, which turns records into that table's rows
//     in ParsedSchema form. Parsers decode their wire format into records and
//     call the builder, so a table fed by several file types (attitude_ahrs
//     from AHRS00 and DFOP00, GNSSObs from NOV000 and NOV770) has one record
//     type and one builder.
//   - Scope and stream EDIDs (Scope, StreamEDID, LogSource).
//
// ParsedSchema is every column of a table except the stamped ones
// (StampedColumns: Network, Station, Campaign, stream_edid), which
// fileparse.Parser.Parse adds afterwards from the scope, plus Log_Source,
// which only the parser knows (it can differ by antenna within one file).
//
// In a record, a nullable column is a pointer field (nil: not logged) and a
// required column is a plain value. Times are time.Time; each record type
// says which time scale. Builders append to the columns they know, by name;
// untouched nullable columns are filled with nulls.
package datamodels
