package convert

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
)

// icebergSource reads the selected rows of each table from a catalog.
type icebergSource struct {
	cfg  catalog.Config
	defs []datamodels.TableDef
	sel  catalog.Selection
	// byStream groups rows by stream and time for an Iceberg sink (see
	// streamGroups) rather than by scan batch.
	byStream bool
}

// newIcebergSource checks spec's Iceberg source values.
func newIcebergSource(spec Spec) (*icebergSource, error) {
	if err := catalog.Check(spec.SourceCatalog); err != nil {
		return nil, usagef("--source-catalog: %w", err)
	}
	if len(spec.SourceURIs) != 1 {
		return nil, usagef("--source %s takes one --source-uri", SourceIceberg)
	}
	if len(spec.Tables) == 0 {
		return nil, usagef("--table is required for --source %s", SourceIceberg)
	}
	if len(spec.SourceTypes) > 1 {
		return nil, usagef("--source-type: --source %s takes one file type or product", SourceIceberg)
	}
	s := &icebergSource{
		cfg: catalog.Config{Name: spec.SourceCatalog, Warehouse: spec.SourceURIs[0]},
		sel: catalog.Selection{Scope: spec.Scope, LogSource: spec.LogSource, Start: spec.Start, End: spec.End, Columns: spec.Columns},
	}
	if len(spec.SourceTypes) == 1 {
		s.sel.FileType = spec.SourceTypes[0]
	}
	if !spec.Start.IsZero() && !spec.End.IsZero() && !spec.Start.Before(spec.End) {
		return nil, usagef("--start %s is not before --end %s", spec.Start.Format(time.RFC3339), spec.End.Format(time.RFC3339))
	}
	for _, name := range spec.Tables {
		def, err := datamodels.TableByName(name)
		if err != nil {
			return nil, usagef("--table: %w", err)
		}
		if _, err := s.sel.Filter(def); err != nil {
			return nil, usagef("%s: %w", def.Name, err)
		}
		sc, err := def.Schema()
		if err != nil {
			return nil, err
		}
		for _, col := range spec.Columns {
			if _, ok := sc.FindFieldByName(col); !ok {
				return nil, usagef("--columns: %s has no column %q", def.Name, col)
			}
		}
		s.defs = append(s.defs, def)
	}
	return s, nil
}

func (s *icebergSource) Read(ctx context.Context, write func(Rows)) error {
	cat, err := catalog.Open(ctx, s.cfg)
	if err != nil {
		return err
	}
	defer cat.Close()
	for _, def := range s.defs {
		if s.byStream {
			schema, groups, err := s.streamGroups(ctx, cat, def)
			write(Rows{Def: def, Schema: schema, Groups: groups, Err: err})
			continue
		}
		schema, batches, err := cat.Scan(ctx, def, s.sel)
		var groups iter.Seq2[Group, error]
		if err == nil {
			groups = func(yield func(Group, error) bool) {
				for rec, err := range batches {
					if err != nil {
						yield(Group{}, err)
						return
					}
					tbl := array.NewTableFromRecords(schema, []arrow.RecordBatch{rec})
					rec.Release()
					if !yield(Group{Rows: tbl}, nil) {
						return
					}
				}
			}
		}
		write(Rows{Def: def, Schema: schema, Groups: groups, Err: err})
	}
	return nil
}

// copyChunk is the time span of one group when copying between catalogs.
const copyChunk = time.Hour

// streamGroups yields the selected rows of def's table one stream at a
// time, in time order, as one group per stream per copyChunk of time. An
// Iceberg sink replaces each group's stream over the group's time span, so
// the groups of one stream must not overlap in time: chunks are
// [t, t+copyChunk), and no time is split across two groups.
func (s *icebergSource) streamGroups(ctx context.Context, cat *catalog.Catalog, def datamodels.TableDef) (*arrow.Schema, iter.Seq2[Group, error], error) {
	// The streams and their time spans, from two columns.
	spanSel := s.sel
	spanSel.Columns = []string{datamodels.StreamEDIDColumn.Name, def.TimeColumn}
	spanSchema, batches, err := cat.Scan(ctx, def, spanSel)
	if err != nil {
		return nil, nil, err
	}
	spans := newStreamTally(def, spanSchema)
	for rec, err := range batches {
		if err != nil {
			return nil, nil, err
		}
		spans.add(rec)
		rec.Release()
	}
	schema, err := def.ArrowSchema()
	if err != nil {
		return nil, nil, err
	}
	groups := func(yield func(Group, error) bool) {
		for _, st := range spans.streams() {
			parts, ok := datamodels.SplitEDID(st.EDID)
			if !ok {
				yield(Group{}, fmt.Errorf("stream_edid %q: want network.station.campaign.type.source", st.EDID))
				return
			}
			sel := s.sel
			sel.Scope = datamodels.Scope{Network: parts[0], Station: parts[1], Campaign: parts[2]}
			sel.FileType, sel.LogSource = parts[3], parts[4]
			for t := st.Start.Truncate(copyChunk); !t.After(st.End); t = t.Add(copyChunk) {
				sel.Start, sel.End = t, t.Add(copyChunk)
				if !s.sel.Start.IsZero() && sel.Start.Before(s.sel.Start) {
					sel.Start = s.sel.Start
				}
				if !s.sel.End.IsZero() && sel.End.After(s.sel.End) {
					sel.End = s.sel.End
				}
				tbl, err := scanTable(ctx, cat, def, sel)
				if err != nil {
					yield(Group{}, fmt.Errorf("%s from %s: %w", st.EDID, sel.Start.Format(time.RFC3339), err))
					return
				}
				if tbl.NumRows() == 0 {
					tbl.Release()
					continue
				}
				if !yield(Group{Rows: tbl}, nil) {
					return
				}
			}
		}
	}
	return schema, groups, nil
}

// scanTable reads the rows sel selects from def's table into one table.
func scanTable(ctx context.Context, cat *catalog.Catalog, def datamodels.TableDef, sel catalog.Selection) (arrow.Table, error) {
	schema, batches, err := cat.Scan(ctx, def, sel)
	if err != nil {
		return nil, err
	}
	var recs []arrow.RecordBatch
	defer func() {
		for _, r := range recs {
			r.Release()
		}
	}()
	for rec, err := range batches {
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return array.NewTableFromRecords(schema, recs), nil
}

// icebergSink appends each group to its table in a catalog as one snapshot,
// replacing each of the group's streams over its time span
// (catalog.Append), so re-running a conversion doesn't duplicate rows.
// Snapshots are committed perCommit at a time (0: all of a table's at
// once). Missing namespaces and tables are created; older schemas are
// evolved.
type icebergSink struct {
	cfg       catalog.Config
	props     map[string]string // added to every snapshot's summary
	perCommit int
	cat       *catalog.Catalog
	openErr   error
}

func (s *icebergSink) Write(ctx context.Context, rows Rows) ([]Output, error) {
	if s.cat == nil && s.openErr == nil {
		s.cat, s.openErr = catalog.Open(ctx, s.cfg)
	}
	if s.openErr != nil {
		return nil, s.openErr
	}
	// The table exists afterwards even if no group has rows.
	batch, err := s.cat.NewBatch(ctx, rows.Def)
	if err != nil {
		return nil, err
	}
	name := rows.Def.Namespace() + "." + rows.Def.Name
	var n int64
	var errs []error
	var sources []string // the staged groups' source files
	// commit commits the staged groups. After a failed commit or append,
	// what was staged is lost: it is reported and a new batch started.
	commit := func() error {
		written, err := batch.Commit(ctx)
		n += written
		if err != nil {
			if len(sources) > 0 {
				err = fmt.Errorf("not committed: %s: %w", strings.Join(sources, ", "), err)
			}
			errs = append(errs, err)
		}
		sources = nil
		if batch.Failed() {
			next, err := s.cat.NewBatch(ctx, rows.Def)
			if err != nil {
				return err
			}
			batch = next
		}
		return nil
	}
	for g, err := range rows.Groups {
		if err != nil {
			errs = append(errs, err)
			break
		}
		opts := []catalog.AppendOption{catalog.WithProperties(s.props)}
		if g.Source != "" {
			opts = append(opts, catalog.WithSource(g.Source))
		}
		_, err := batch.Append(ctx, g.Rows, opts...)
		g.Rows.Release()
		if err != nil {
			if g.Source != "" {
				err = fmt.Errorf("%s: %w", g.Source, err)
			}
			errs = append(errs, err)
			if batch.Failed() {
				if err := commit(); err != nil {
					errs = append(errs, err)
					break
				}
			}
			continue
		}
		if g.Source != "" {
			sources = append(sources, g.Source)
		}
		if s.perCommit > 0 && batch.Staged() >= s.perCommit {
			if err := commit(); err != nil {
				errs = append(errs, err)
				break
			}
		}
	}
	if err := commit(); err != nil {
		errs = append(errs, err)
	}
	var outs []Output
	if n > 0 || len(errs) == 0 {
		outs = []Output{{Kind: "iceberg", Table: name, Rows: n}}
	}
	return outs, errors.Join(errs...)
}

// Close closes the catalog, if Write opened it.
func (s *icebergSink) Close() error {
	if s.cat == nil {
		return nil
	}
	return s.cat.Close()
}
