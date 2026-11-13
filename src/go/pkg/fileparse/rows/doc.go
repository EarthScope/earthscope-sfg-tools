// Package rows turns parsed records into Arrow tables with a table's full
// schema (datamodels.TableDef), one record type and builder per table: XRow,
// e.g. INSPVARow, and BuildX. Parsers decode their wire format into records
// and call the builder, so a table fed by several file types (attitude_ahrs
// from AHRS00 and DFOP00, gnss_observations from NOV000 and NOV770) has one
// record type and one builder.
//
// Each builder takes the stream EDID of its rows (package edid), which the
// parser computes from the scope, its file type and the rows' log source.
//
// In a record, a nullable column is a pointer field (nil: not logged) and a
// required column is a plain value. Times are time.Time; each record type
// says which time scale. Builders append to the columns they know, by name;
// untouched nullable columns are filled with nulls.
package rows
