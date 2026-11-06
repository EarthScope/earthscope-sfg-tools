// Package datamodels is the data model of the Level 0 tables, shared by the
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
// Every table's only identity column is stream_edid,
// network.station.campaign.file_type.log_source, and it is partitioned on
// it. ParsedSchema is every column of a table except stream_edid
// (StampedColumns), which fileparse.Parser.Parse adds afterwards from the
// scope and each row's Log_Source. Only the parser knows Log_Source (it can
// differ by antenna within one file); no table stores it.
//
// In a record, a nullable column is a pointer field (nil: not logged) and a
// required column is a plain value. Times are time.Time; each record type
// says which time scale. Builders append to the columns they know, by name;
// untouched nullable columns are filled with nulls.
package datamodels
