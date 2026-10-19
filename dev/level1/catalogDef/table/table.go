package table

import (
	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
)

type Table struct {
	Name      string
	Table     *icetable.Table
	WhLoc     string                // Warehouse location suffix
	Partition iceberg.PartitionSpec // Warehouse partition spec
	Schema    *iceberg.Schema
}
