package convert

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
)

// Run connects one Reader to one Writer: it reads what sel selects and writes
// each Batch, and returns the Result, which holds each input, stream and
// output and every error. It describes only what was read and written; the
// caller adds what was asked for (Result.Source, Sink, Selection).
//
// Before reading, Run returns a *UsageError when the Writer does not accept a
// table the run would write (those of sel.Tables, or all the Reader's). After
// that a Batch that fails is recorded (its input is "failed", or "skipped"
// for storage.ErrSkipped) and the run goes on, so a run where some batches
// failed and others were written is ExitPartial (see ExitCode). A returned
// error is a fatal one, after which the Result holds what was done before it.
// A batch whose Write fails is a failed batch too, and its input is failed.
// A Reader that yields no batch at all, neither data nor an error, has
// matched nothing: the Result records a "read" error ("selection matched no
// rows") and the run is fatal. So is a run whose every input was skipped
// ("no recognised input files"). A raw file that parses to zero rows still
// yields a batch, so it is a success. A Writer whose Close fails is a
// "sink" error: fatal when nothing was written, else partial (ExitCode).
// The Writer is closed before Run returns; a Writer with an
// Outputs() []storage.Output method has those added to the Result after Close.
// A Writer reports what it wrote through Write's return value or Outputs(),
// not both: Run appends both.
func Run(ctx context.Context, r storage.Reader, w storage.Writer, sel storage.Selection) (*Result, error) {
	tables := r.Tables()
	refs := sel.Tables
	if len(refs) == 0 {
		refs = tables
	}
	for _, ref := range refs {
		if !w.Accepts(ref) {
			w.Close()
			return nil, usagef("the sink does not accept table %s", ref)
		}
	}
	res := &Result{Schema: ResultSchema, Command: "convert", Scope: scopeString(sel.Scope),
		Inputs: []Input{}, Streams: []Stream{}, Outputs: []Output{}, Errors: []Error{}}
	typer, _ := r.(interface{ FileType(path string) string })

	inputAt := map[string]int{}        // path -> index in res.Inputs
	errs := map[string][]string{}      // path -> parse failures
	writeErrs := map[string][]string{} // path -> write failures
	tallies := map[string]*streamTally{}
	var order []string // tables, as first seen
	err := r.Read(ctx, sel, func(b storage.Batch) error {
		in := -1
		if b.Source != nil {
			i, ok := inputAt[b.Source.Path]
			if !ok {
				i = len(res.Inputs)
				inputAt[b.Source.Path] = i
				res.Inputs = append(res.Inputs, Input{Path: b.Source.Path, Status: "ok"})
				if typer != nil {
					res.Inputs[i].Type = typer.FileType(b.Source.Path)
				}
			}
			in = i
		}
		switch {
		case errors.Is(b.Err, storage.ErrSkipped):
			if in >= 0 {
				res.Inputs[in].Status, res.Inputs[in].Error = "skipped", b.Err.Error()
			}
			return nil
		case b.Err != nil:
			res.failed++
			if in >= 0 {
				errs[b.Source.Path] = append(errs[b.Source.Path], b.Err.Error())
			} else {
				res.addError("read", b.Ref.String(), b.Err)
			}
			return nil
		}
		defer b.Table.Release()
		name := b.Ref.String()
		t, ok := tallies[name]
		if !ok {
			t = newStreamTally(b.Ref, b.Table.Schema())
			tallies[name] = t
			order = append(order, name)
		}
		t.addTable(b.Table)
		outs, err := w.Write(ctx, b)
		res.Outputs = append(res.Outputs, outputs(outs)...)
		if err != nil {
			res.failed++
			res.addError("write", name, err)
			if in >= 0 {
				writeErrs[b.Source.Path] = append(writeErrs[b.Source.Path], name+": write: "+err.Error())
			}
		} else {
			res.succeeded++
		}
		return nil
	})
	batches := res.succeeded + res.failed
	switch {
	case err != nil || batches > 0:
	case len(res.Inputs) == 0:
		res.addError("read", "", errors.New("selection matched no rows"))
	case !slices.ContainsFunc(res.Inputs, func(in Input) bool { return in.Status != "skipped" }):
		res.addError("read", "", errors.New("no recognised input files"))
	}
	// A file that failed for any table, in parsing or writing, is a failed
	// input; a parse failure is also an error of its own (write failures
	// were recorded as they happened).
	for path, msgs := range errs {
		i := inputAt[path]
		res.Inputs[i].Status, res.Inputs[i].Error = "failed", strings.Join(append(msgs, writeErrs[path]...), "; ")
	}
	for path, msgs := range writeErrs {
		if i := inputAt[path]; len(errs[path]) == 0 {
			res.Inputs[i].Status, res.Inputs[i].Error = "failed", strings.Join(msgs, "; ")
		}
	}
	for _, in := range res.Inputs {
		if in.Status == "failed" && len(errs[in.Path]) > 0 {
			res.Errors = append(res.Errors, Error{Stage: "parse", Subject: in.Path, Message: in.Error})
		}
	}
	// Streams by table in the Reader's order, then by stream.
	pos := func(name string) int {
		for i, ref := range tables {
			if ref.String() == name {
				return i
			}
		}
		return len(tables)
	}
	slices.SortStableFunc(order, func(a, b string) int { return pos(a) - pos(b) })
	for _, name := range order {
		res.Streams = append(res.Streams, tallies[name].streams()...)
	}
	if err != nil {
		res.addError("source", "", err)
	}
	if cerr := w.Close(); cerr != nil {
		res.addError("sink", "", cerr)
	}
	if o, ok := w.(interface{ Outputs() []storage.Output }); ok {
		res.Outputs = append(res.Outputs, outputs(o.Outputs())...)
	}
	res.OK = len(res.Errors) == 0
	return res, err
}

// outputs are what a Writer wrote, as the Result records them.
func outputs(outs []storage.Output) []Output {
	var out []Output
	for _, o := range outs {
		kind := o.Kind
		if kind == "" {
			kind = "file"
		}
		out = append(out, Output{Kind: kind, Path: o.Path, Table: o.Ref.String(), Rows: o.Rows, Bytes: o.Bytes})
	}
	return out
}
