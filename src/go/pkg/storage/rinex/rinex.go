// Package rinex is the RINEX Writer: it takes the gnss_observations Batches
// of one stream, converts them back to epochs (ToEpochs) and, at Close,
// writes one daily RINEX observation file per day. It accepts only
// datamodels.L0GNSSObs. Layering: it imports storage, catalog, datamodels
// and gnsstools, not convert or cmd.
package rinex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"gitlab.com/earthscope/gnsstools/core/gnss/observation"

	gnssrinex "gitlab.com/earthscope/gnsstools/codecs/rinex"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/storage"
)

// Writer buffers the epochs of the Batches it is given and writes them, at
// Close, as daily RINEX observation files in one directory: the epochs are
// ordered and de-duplicated by time, flagged for cycle slips from the lock
// times, optionally decimated, split by calendar day and written one file
// per day (named by RINEX 3 convention). The whole run's epochs are held in
// memory, since a RINEX 3/4 header needs every epoch before the first is
// written.
//
// A RINEX file holds one antenna's stream. When a Batch's stream differs
// from the first one seen, Write returns an error wrapping
// ErrMultipleStreams that names both, the Writer takes no more Batches
// (Conflict returns the error) and Close writes nothing.
//
// Write returns no outputs; the files are reported by Outputs, after Close.
type Writer struct {
	dir      string
	settings *gnssrinex.Settings
	modulo   int64

	mu       sync.Mutex
	stream   string
	epochs   []observation.Epoch
	conflict error
	closed   bool
	outs     []storage.Output
}

// NewWriter makes a Writer of RINEX files into dir, which is created at Close
// when there is something to write. settings supplies the file header;
// moduloMillis > 0 keeps only the epochs on a multiple of it (1000 for 1 Hz).
func NewWriter(dir string, settings *gnssrinex.Settings, moduloMillis int64) *Writer {
	return &Writer{dir: dir, settings: settings, modulo: moduloMillis}
}

// Accepts is true only for gnss_observations.
func (w *Writer) Accepts(ref catalog.TableRef) bool {
	return ref.Database == datamodels.L0GNSSObs.Database && ref.Def.Name == datamodels.L0GNSSObs.Def.Name
}

// Write converts b's rows to epochs and holds them until Close.
func (w *Writer) Write(_ context.Context, b storage.Batch) ([]storage.Output, error) {
	if b.Table == nil {
		return nil, fmt.Errorf("%s: batch has no table", b.Ref)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case w.closed:
		return nil, fmt.Errorf("%s: write after close", b.Ref)
	case w.conflict != nil:
		return nil, w.conflict
	}
	stream, epochs, err := ToEpochs(b.Table)
	if err != nil {
		if errors.Is(err, ErrMultipleStreams) {
			w.conflict = err
			w.epochs = nil
		}
		return nil, err
	}
	switch {
	case len(epochs) == 0:
		return nil, nil
	case w.stream == "":
		w.stream = stream
	case stream != w.stream:
		w.conflict = fmt.Errorf("%w: %s and %s", ErrMultipleStreams, w.stream, stream)
		w.epochs = nil
		return nil, w.conflict
	}
	w.epochs = append(w.epochs, epochs...)
	return nil, nil
}

// Conflict is the error wrapping ErrMultipleStreams when the Batches held
// more than one stream, else nil.
func (w *Writer) Conflict() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conflict
}

// Close writes the files, unless a stream conflict happened (nothing is
// written then and Close returns nil: Write reported it). It is an error to
// have no epochs to write. A file that cannot be written is removed and its
// error returned; the other days' files stay.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.conflict != nil {
		return nil
	}
	epochs := prepare(w.epochs, w.modulo)
	w.epochs = nil
	if len(epochs) == 0 {
		return errors.New("no GNSS observation epochs to write")
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	var errs []error
	for _, day := range byDay(epochs) {
		path, err := writeFile(w.dir, w.settings, day)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out := storage.Output{Ref: datamodels.L0GNSSObs, Path: path, Rows: int64(len(day))}
		if info, err := os.Stat(path); err == nil {
			out.Bytes = info.Size()
		}
		w.outs = append(w.outs, out)
	}
	return errors.Join(errs...)
}

// Outputs are the files written, once Close has written them; each Output's
// Rows is its epochs.
func (w *Writer) Outputs() []storage.Output {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]storage.Output(nil), w.outs...)
}
