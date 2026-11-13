package datamodels

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"gitlab.com/earthscope/gnsstools/geodata/gnssiceberg"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
)

// gnsstoolsObservations is GNSSObs's External definition: exactly what
// gnsstools creates for its observations table. gnsstools does not export
// the definition, only EnsureObservationsTable, so the table is created once
// per process in a throwaway SQL catalog and its definition copied out.
var gnsstoolsObservations = sync.OnceValues(func() (catalog.External, error) {
	dir, err := os.MkdirTemp("", "gnssobs-schema-")
	if err != nil {
		return catalog.External{}, err
	}
	defer os.RemoveAll(dir)
	tbl, err := gnssiceberg.EnsureObservationsTable(context.Background(),
		gnssiceberg.CatalogConfig{Type: gnssiceberg.CatalogTypeSQL, URI: "file:" + filepath.Join(dir, "catalog.db")},
		gnssiceberg.TableConfig{DatabaseName: "probe", TableName: "observations", WarehouseLocation: dir},
	)
	if err != nil {
		return catalog.External{}, fmt.Errorf("gnsstools observations table definition: %w", err)
	}
	return catalog.External{
		Schema:     tbl.Schema(),
		Spec:       tbl.Spec(),
		SortOrder:  tbl.SortOrder(),
		Properties: maps.Clone(tbl.Properties()),
	}, nil
})
