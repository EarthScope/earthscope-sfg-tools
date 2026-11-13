// Package datamodels declares the SFG tables: what exists, not how it is
// filled or where it lives.
//
//   - TableDef: one table's name, Iceberg schema, checks, partitioning, sort
//     order and properties, e.g. INSPVA, TWTT, GNSSObs, CommunityStandard.
//   - DatabaseDef: a database (Iceberg namespace) and the tables in it. A
//     table's level is the database it is in (gnssa_l0, gnssa_l1,
//     gnssa_l2); a definition can be in several, e.g. community_standard.
//   - CatalogDef: the databases of a catalog; SFG is the seafloor geodesy
//     catalog. A TableRef names one table of it: a definition in a database.
//
// Every table's only identity column is stream_edid (StreamEDIDName), and
// it is partitioned on it; package edid builds its values. Rows are built
// by the parsers (package fileparse/rows) and written by package catalog,
// which builds these definitions in a local warehouse or AWS Glue.
package datamodels
