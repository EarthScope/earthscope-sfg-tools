package storage

import (
	"errors"
	"math"
	"strings"

	"github.com/apache/iceberg-go"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/catalog"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// Selection picks the rows to read. Scope.Network is required; Station and
// Campaign narrow it, and Campaign needs Station.
//
// Tables are the tables to read (empty: all the Reader has). LogSources pick
// streams (see edid.Stream) and need the full scope: a LogSource with a Name
// is one stream, one with only a FileType all that file type's streams;
// empty means every stream of the scope. In a derived table (Level 1/2) the
// FileType is the product and the Name its input (see edid.Derived), e.g.
// PRIDE and NOV000:RANGEA. Scope and LogSources both match a prefix of
// stream_edid, the table's only identity column, which it is partitioned on.
//
// Window bounds the table's TimeColumn, both ends inclusive; nil is
// unbounded, and Min of math.MinInt64 or Max of math.MaxInt64 leaves that
// end open. For gnss_observations the time column is time_gpst, GPS time.
// Columns are the columns to read, in this order; all, in table order, when
// empty.
type Selection struct {
	Scope      edid.Scope
	Tables     []catalog.TableRef
	LogSources []edid.LogSource
	Columns    []string
	Window     *catalog.Window
}

// streamTerm is one stream_edid test of a Selection: a prefix (ending in
// edid.Sep) or, with exact, one whole stream.
type streamTerm struct {
	value string
	exact bool
}

// streamTerms checks the scope and log sources and returns the stream_edid
// tests they stand for, any one of which selects a row.
func (s Selection) streamTerms() ([]streamTerm, error) {
	sc := s.Scope
	switch {
	case sc.Network == "":
		return nil, errors.New("selection needs a network")
	case sc.Campaign != "" && sc.Station == "":
		return nil, errors.New("selection by campaign needs a station")
	case len(s.LogSources) > 0 && sc.Campaign == "":
		return nil, errors.New("selection by log source needs the full scope (network, station, campaign)")
	}
	if len(s.LogSources) == 0 {
		prefix := sc.Network + edid.Sep
		for _, p := range []string{sc.Station, sc.Campaign} {
			if p != "" {
				prefix += p + edid.Sep
			}
		}
		return []streamTerm{{value: prefix}}, nil
	}
	var terms []streamTerm
	for _, ls := range s.LogSources {
		switch {
		case ls.FileType == "":
			return nil, errors.New("selection by log source needs its file type")
		case ls.Name == "":
			terms = append(terms, streamTerm{value: edid.Derived(sc, ls.FileType, "")})
		default:
			terms = append(terms, streamTerm{value: edid.Derived(sc, ls.FileType, ls.Name), exact: true})
		}
	}
	return terms, nil
}

// StreamMatcher is the Scope and LogSources of s as a test on a stream_edid,
// for a Reader that filters rows itself. It reports the same errors as
// Filter does.
func (s Selection) StreamMatcher() (func(stream string) bool, error) {
	terms, err := s.streamTerms()
	if err != nil {
		return nil, err
	}
	return func(stream string) bool {
		for _, t := range terms {
			if t.exact && stream == t.value || !t.exact && strings.HasPrefix(stream, t.value) {
				return true
			}
		}
		return false
	}, nil
}

// Filter is sel as a row filter on the table def.
func (s Selection) Filter(def catalog.TableDef) (iceberg.BooleanExpression, error) {
	terms, err := s.streamTerms()
	if err != nil {
		return nil, err
	}
	ed := iceberg.Reference(catalog.StreamEDIDName)
	var ors []iceberg.BooleanExpression
	for _, t := range terms {
		if t.exact {
			ors = append(ors, iceberg.EqualTo(ed, t.value))
		} else {
			ors = append(ors, hasPrefix(ed, t.value))
		}
	}
	var preds []iceberg.BooleanExpression
	if len(ors) == 1 {
		preds = append(preds, ors[0])
	} else {
		preds = append(preds, iceberg.NewOr(ors[0], ors[1], ors[2:]...))
	}

	if w := s.Window; w != nil {
		if w.Min != math.MinInt64 {
			preds = append(preds, iceberg.GreaterThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.Min)))
		}
		if w.Max != math.MaxInt64 {
			preds = append(preds, iceberg.LessThanEqual(iceberg.Reference(def.TimeColumn), iceberg.Timestamp(w.Max)))
		}
	}
	if len(preds) == 1 {
		return preds[0], nil
	}
	return iceberg.NewAnd(preds[0], preds[1], preds[2:]...), nil
}

// hasPrefix matches strings starting with prefix, which ends in edid.Sep.
// It is the range [prefix, prefix with its last byte raised by one), since
// iceberg-go scans do not implement starts_with; the range also prunes data
// files by their column bounds.
func hasPrefix(ref iceberg.UnboundTerm, prefix string) iceberg.BooleanExpression {
	upper := prefix[:len(prefix)-1] + string(prefix[len(prefix)-1]+1)
	return iceberg.NewAnd(iceberg.GreaterThanEqual(ref, prefix), iceberg.LessThan(ref, upper))
}
