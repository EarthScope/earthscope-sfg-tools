// Command sfglevel1 parses one raw GNSS-A log into a full-row Level 1 Arrow
// IPC file per table, for the Python ingest layer (dev/iceberg_level1.py).
//
// Usage:
//
//	sfglevel1 FILETYPE -out DIR [-table NAME] FILE
//
// FILETYPE is one of fileparse.FileTypes (AHRS00, NOV770, NOV000, DFOP00).
// Every table the file type produces is written as DIR/<table>.arrow, or
// only -table NAME. The scope is read from FILE's
// .../<network>/<station>/<campaign>/raw directory unless -network, -station
// and -campaign override it.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"

	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/catalogDef/schemas"
	"github.com/EarthScope/earthscope-sfg-tools/dev/level1/fileparse"
)

const usage = "usage: sfglevel1 FILETYPE -out DIR [-table NAME] FILE"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "sfglevel1:", err)
		os.Exit(1)
	}
}

func run(fileType string, args []string) error {
	ft, ok := fileparse.Lookup(fileType)
	if !ok {
		return fmt.Errorf("unknown file type %q", fileType)
	}
	fs := flag.NewFlagSet(ft.Name, flag.ExitOnError)
	out := fs.String("out", "", "output directory (required)")
	tableName := fs.String("table", "", "write only this table (default: all the file type produces)")
	network := fs.String("network", "", "Network scope (default: from the file path)")
	station := fs.String("station", "", "Station scope (default: from the file path)")
	campaign := fs.String("campaign", "", "Campaign scope (default: from the file path)")
	fs.Parse(args)
	if *out == "" || fs.NArg() != 1 {
		return fmt.Errorf(usage)
	}
	file := fs.Arg(0)
	scope := schemas.ScopeFromRawDir(filepath.Dir(file))
	for _, o := range []struct{ dst, v *string }{
		{&scope.Network, network}, {&scope.Station, station}, {&scope.Campaign, campaign},
	} {
		if *o.v != "" {
			*o.dst = *o.v
		}
	}
	defs := ft.Tables
	if *tableName != "" {
		def, ok := ft.Table(*tableName)
		if !ok {
			return fmt.Errorf("%s files do not produce table %q", ft.Name, *tableName)
		}
		defs = []schemas.TableDef{def}
	}
	for _, def := range defs {
		tbl, err := ft.Parse(file, def, scope)
		if err != nil {
			return err
		}
		slog.Info("parsed", "file", file, "table", def.Name, "rows", tbl.NumRows())
		err = writeArrowIPC(filepath.Join(*out, def.Name+".arrow"), tbl)
		tbl.Release()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeArrowIPC(path string, tbl arrow.Table) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := ipc.NewFileWriter(f, ipc.WithSchema(tbl.Schema()))
	if err != nil {
		return fmt.Errorf("opening Arrow IPC writer for %s: %w", path, err)
	}
	rdr := array.NewTableReader(tbl, -1)
	defer rdr.Release()
	for rdr.Next() {
		if err := w.Write(rdr.RecordBatch()); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", path, err)
	}
	return f.Close()
}
