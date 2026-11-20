// Package raw is the Reader of raw instrument files, built on the parser
// registry (fileparse.Parsers): each file is parsed once per table its type
// feeds, and each parse is one Batch.
package raw

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
)

// dirPattern is the base-name glob read when an input is a campaign raw
// directory, relative to it: fileparse.Patterns, except that QC PIN files
// are unpacked under the campaign's qc/, beside the raw directory. NVA has no
// pattern, so a directory never yields it.
func dirPattern(fileType string) string {
	if fileType == fileparse.FileTypeQCPIN {
		return "../qc/*/*.pin"
	}
	return fileparse.Patterns[fileType]
}

// Reader reads the raw files at its inputs.
type Reader struct {
	inputs   []string
	types    []string // upper-case file types to read; nil: all
	asGiven  bool     // named files are of the one type in types, whatever their name
	maxFiles int
	workers  int

	mu    sync.Mutex
	typeA map[string]string // path -> file type, of the files Read chose
}

// Option changes how a Reader reads.
type Option func(*Reader)

// Types limits the Reader to the files of these file types (case-insensitive
// names, e.g. "DFOP00"): a directory input yields only their files, and a
// named file or glob match is read only if its name matches one of them.
func Types(types ...string) Option {
	return func(r *Reader) {
		for _, t := range types {
			r.types = append(r.types, strings.ToUpper(t))
		}
	}
}

// AsGiven makes a named file or glob match (not a directory's) of the one
// file type given with Types, whatever its name.
func AsGiven() Option { return func(r *Reader) { r.asGiven = true } }

// MaxFiles reads at most n files of each type, in sorted order; 0 is all.
func MaxFiles(n int) Option { return func(r *Reader) { r.maxFiles = n } }

// Workers parses n files at once; 0 is one per CPU.
func Workers(n int) Option { return func(r *Reader) { r.workers = n } }

// NewReader makes a Reader of inputs: files, globs or directories. A
// directory yields the files matching each file type's name pattern; a file
// or glob match is of the file type its name matches (see
// fileparse.FileTypeOf), and a file of none is yielded as a Batch with
// storage.ErrSkipped. Files are read in sorted order, each once however
// often it is named, skipping the "._" files macOS writes on non-Mac
// volumes. It fails on no inputs, an invalid glob, or an Option naming a
// file type without parsers; the files themselves are found by Read.
func NewReader(inputs []string, opts ...Option) (*Reader, error) {
	if len(inputs) == 0 {
		return nil, errors.New("raw reader needs at least one input")
	}
	r := &Reader{inputs: inputs, typeA: map[string]string{}}
	for _, o := range opts {
		o(r)
	}
	if r.maxFiles < 0 {
		return nil, fmt.Errorf("max files %d: want 0 (all) or more", r.maxFiles)
	}
	if r.workers < 0 {
		return nil, fmt.Errorf("workers %d: want 0 (one per CPU) or more", r.workers)
	}
	if r.workers == 0 {
		r.workers = runtime.NumCPU()
	}
	for i, t := range r.types {
		ps := fileparse.ForFileType(t)
		if len(ps) == 0 {
			return nil, fmt.Errorf("file type %q: no parsers", t)
		}
		r.types[i] = ps[0].FileType
	}
	if r.asGiven && len(r.types) != 1 {
		return nil, errors.New("AsGiven needs exactly one file type")
	}
	for _, in := range inputs {
		if _, err := filepath.Match(in, ""); err != nil {
			return nil, fmt.Errorf("input %s: %w", in, err)
		}
	}
	return r, nil
}

// Tables are the tables the Reader's file types feed, in declaration order.
func (r *Reader) Tables() []catalog.TableRef {
	var out []catalog.TableRef
	for _, ref := range datamodels.GNSSA_L0.Refs() {
		for _, p := range fileparse.Parsers {
			if sameRef(p.Ref, ref) && r.reads(p.FileType) {
				out = append(out, ref)
				break
			}
		}
	}
	return out
}

func sameRef(a, b catalog.TableRef) bool { return a.Database == b.Database && a.Def.Name == b.Def.Name }

func (r *Reader) reads(fileType string) bool {
	return r.types == nil || slices.Contains(r.types, fileType)
}

// FileType is the file type of the file at path, if Read chose it, else "".
func (r *Reader) FileType(path string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.typeA[path]
}

type file struct{ path, fileType string }

// files expands the inputs: the files to parse by type then path, and the
// named files of no type.
func (r *Reader) files() (found []file, skipped []string, err error) {
	byType := map[string][]string{}
	types := r.types
	if types == nil {
		for _, p := range fileparse.Parsers {
			if !slices.Contains(types, p.FileType) {
				types = append(types, p.FileType)
			}
		}
	}
	for _, in := range r.inputs {
		info, serr := os.Stat(in)
		var named []string
		switch {
		case serr == nil && info.IsDir():
			for _, ft := range types {
				if pat := dirPattern(ft); pat != "" {
					matches, _ := filepath.Glob(filepath.Join(in, pat))
					byType[ft] = append(byType[ft], matches...)
				}
			}
			continue
		case strings.ContainsAny(in, "*?["):
			if named, err = filepath.Glob(in); err != nil {
				return nil, nil, fmt.Errorf("input %s: %w", in, err)
			}
		case serr != nil:
			return nil, nil, serr
		default:
			named = []string{in}
		}
		for _, path := range named {
			ft := ""
			switch {
			case r.asGiven:
				ft = r.types[0]
			default:
				if ft = fileparse.FileTypeOf(path); ft != "" && !slices.Contains(types, ft) {
					ft = ""
				}
			}
			if ft == "" {
				skipped = append(skipped, path)
				continue
			}
			byType[ft] = append(byType[ft], path)
		}
	}
	// Directories and the "._" AppleDouble files macOS writes beside files
	// on non-Mac volumes are not files to parse.
	drop := func(p string) bool {
		fi, err := os.Stat(p)
		return strings.HasPrefix(filepath.Base(p), "._") || (err == nil && fi.IsDir())
	}
	for _, ft := range types {
		paths := slices.DeleteFunc(byType[ft], drop)
		slices.Sort(paths)
		paths = slices.Compact(paths)
		if r.maxFiles > 0 && len(paths) > r.maxFiles {
			paths = paths[:r.maxFiles]
		}
		for _, p := range paths {
			found = append(found, file{p, ft})
		}
	}
	skipped = slices.DeleteFunc(skipped, drop)
	slices.Sort(skipped)
	skipped = slices.Compact(skipped)
	if len(found) == 0 && len(skipped) == 0 {
		return nil, nil, fmt.Errorf("no %s files at %s", strings.Join(types, ", "), strings.Join(r.inputs, " "))
	}
	return found, skipped, nil
}

// job is one file to parse with one parser.
type job struct {
	path string
	p    fileparse.Parser
	src  edid.LogSource // the log source to parse, or edid.AllLogSources
}

// jobs lists what to parse for found: each file with each parser of its type
// whose table is in sel.Tables (all when empty). Per file type, the entries of
// sel.LogSources of that type with a name pick the log sources: a parser is
// run once per entry its table lists, and not at all if none is. With no such
// entry the parser is run once for every log source.
func jobs(found []file, sel storage.Selection) []job {
	var out []job
	for _, f := range found {
		var named []edid.LogSource
		for _, ls := range sel.LogSources {
			if ls.FileType == f.fileType && ls.Name != "" {
				named = append(named, ls)
			}
		}
		for _, p := range fileparse.ForFileType(f.fileType) {
			if len(sel.Tables) > 0 && !slices.ContainsFunc(sel.Tables, func(t catalog.TableRef) bool { return sameRef(t, p.Ref) }) {
				continue
			}
			if len(named) == 0 {
				out = append(out, job{f.path, p, edid.AllLogSources})
				continue
			}
			for _, ls := range named {
				if slices.Contains(p.Sources(), ls) {
					out = append(out, job{f.path, p, ls})
				}
			}
		}
	}
	return out
}

// Read parses the files, up to the Reader's workers at once and at most
// twice that many parsed files waiting to be taken, and yields one Batch per
// parse in file order. Files of no known type come first, as Batches with
// ErrSkipped; a file that fails to parse is a Batch with its error and the
// run goes on.
//
// Of sel it honours Tables (the parsers run), LogSources with a Name (the
// streams parsed; see jobs) and Scope, which stamps every row and is not
// checked against the files. It returns an error, before reading, for what
// it cannot honour: a Window, Columns, or a LogSource without a Name.
func (r *Reader) Read(ctx context.Context, sel storage.Selection, yield func(storage.Batch) error) error {
	switch {
	case sel.Window != nil:
		return errors.New("raw reader: a selection by time window is not supported")
	case len(sel.Columns) > 0:
		return errors.New("raw reader: a selection of columns is not supported")
	}
	for _, ls := range sel.LogSources {
		if ls.Name == "" {
			return fmt.Errorf("raw reader: log source %q has no name: select one stream (file type and name)", ls.FileType)
		}
	}
	found, skipped, err := r.files()
	if err != nil {
		return err
	}
	r.mu.Lock()
	for _, f := range found {
		r.typeA[f.path] = f.fileType
	}
	r.mu.Unlock()
	for _, path := range skipped {
		if err := yield(storage.Batch{Source: &catalog.SourceFile{Path: path}, Err: storage.ErrSkipped}); err != nil {
			return err
		}
	}
	js := jobs(found, sel)

	ctx, cancel := context.WithCancel(ctx)
	results := make([]chan storage.Batch, len(js))
	for i := range results {
		results[i] = make(chan storage.Batch, 1)
	}
	ahead := make(chan struct{}, 2*r.workers) // parsing or parsed, not yet taken
	busy := make(chan struct{}, r.workers)    // parsing
	var wg sync.WaitGroup
	dispatched := make(chan struct{})
	go func() {
		defer close(dispatched)
		for i, j := range js {
			select {
			case ahead <- struct{}{}:
			case <-ctx.Done():
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				busy <- struct{}{}
				var b storage.Batch
				if err := ctx.Err(); err != nil {
					b = storage.Batch{Ref: j.p.Ref, Err: err}
				} else {
					b = j.run(sel.Scope)
				}
				<-busy
				results[i] <- b
			}()
		}
	}()
	// On return, stop dispatching, wait for the parses under way and release
	// what nobody took.
	defer func() {
		cancel()
		<-dispatched
		wg.Wait()
		for _, ch := range results {
			select {
			case b := <-ch:
				if b.Table != nil {
					b.Table.Release()
				}
			default:
			}
		}
	}()
	for i := range js {
		var b storage.Batch
		select {
		case b = <-results[i]:
		case <-ctx.Done():
			return ctx.Err()
		}
		<-ahead
		if b.Err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := yield(b); err != nil {
			return err // yield owns b.Table, whatever it returns
		}
	}
	return nil
}

// run parses j's file (only j.src's rows if set) and hashes it.
func (j job) run(scope edid.Scope) storage.Batch {
	fail := func(err error) storage.Batch {
		return storage.Batch{Ref: j.p.Ref, Source: &catalog.SourceFile{Path: j.path},
			Err: fmt.Errorf("%s: %w", j.p.Ref.Def.Name, err)}
	}
	tbl, err := j.p.Parse(j.path, j.src, scope)
	if err != nil {
		return fail(err)
	}
	src, err := catalog.HashFile(j.path)
	if err != nil {
		tbl.Release()
		return fail(err)
	}
	return storage.Batch{Ref: j.p.Ref, Table: tbl, Source: &src}
}
