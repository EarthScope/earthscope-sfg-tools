// Package storage is the two interfaces data moves through, and the types
// they exchange: a Reader yields Batches of rows, a Writer takes them.
// Implementations live in subpackages (raw, files); nothing here knows how
// a backend stores its rows. convert connects one Reader to one Writer.
//
// Layering: storage and its subpackages do not import convert or cmd.
package storage

import (
	"context"
	"errors"

	"github.com/apache/arrow-go/v18/arrow"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
)

// ErrSkipped is the Err of a Batch for a raw file whose type no parser
// recognises: it is skipped, not a failure.
var ErrSkipped = errors.New("not a recognised raw file")

// A Batch is one piece of one table: a Reader yields many per table, one per
// raw file (and parser) or one per stream-hour of an Iceberg table.
//
// A Batch with Err set carries no Table: it reports that its piece (the
// file at Source.Path, when Source is set) could not be read, and the run
// goes on. Ref is then the table that was being read, if known.
//
// Ownership of a Batch's Table passes to yield when a Reader calls it,
// whatever yield returns: the Reader must not use or release the Table
// after the call, and yield releases it (convert.Run does so once the Writer
// returns). A Writer that keeps the Table beyond Write must Retain it.
type Batch struct {
	Ref    catalog.TableRef
	Table  arrow.Table
	Source *catalog.SourceFile // the raw file's path and SHA-256; nil when not from a raw file
	Err    error
}

// A Reader yields the rows a Selection picks.
type Reader interface {
	// Tables are the tables the Reader can yield.
	Tables() []catalog.TableRef
	// Read calls yield once per Batch, in order, and stops at the first
	// error yield returns (and returns it). yield owns each Batch's Table
	// (see Batch). An error from Read itself means
	// the source could not be read at all; a failure confined to one piece
	// is a Batch with Err instead.
	Read(ctx context.Context, sel Selection, yield func(Batch) error) error
}

// A Writer takes the Batches of a run.
type Writer interface {
	// Accepts reports whether the Writer can write ref's table.
	Accepts(ref catalog.TableRef) bool
	// Write writes one Batch (many are written per table) and returns what
	// it wrote now, which may be nothing for a Writer that holds batches
	// until Close. A Writer reports its outputs through this return value or
	// through an Outputs() []Output method that convert.Run calls after
	// Close, not both.
	Write(ctx context.Context, b Batch) ([]Output, error)
	// Close finishes the run. A Writer that holds data until Close writes it
	// here.
	Close() error
}

// An Output is something a Writer wrote.
type Output struct {
	Ref   catalog.TableRef
	Kind  string // "file" when empty; the Iceberg Writer's is "iceberg"
	Path  string
	Rows  int64
	Bytes int64
}
