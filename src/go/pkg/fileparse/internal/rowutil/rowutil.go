// Package rowutil holds the small helpers the format packages share when they
// build rows: the log source filter every parse function applies and an
// optional-string constructor.
package rowutil

import "github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"

// Filter returns the rows whose log source is src, or all of rows when src is
// edid.AllLogSources. logSource reads a row's LogSource.
func Filter[R any](rows []R, src edid.LogSource, logSource func(R) edid.LogSource) []R {
	if src.IsAll() {
		return rows
	}
	var out []R
	for _, r := range rows {
		if logSource(r) == src {
			out = append(out, r)
		}
	}
	return out
}

// OptString is &v, or nil when v is empty.
func OptString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
