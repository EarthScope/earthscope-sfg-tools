// Package logtype checks the log type a file parser is called with.
package logtype

import (
	"fmt"
	"slices"
)

// Check returns an error unless logType is one of valid, the log types a
// parser of fileType files reads.
func Check(fileType, logType string, valid ...string) error {
	if slices.Contains(valid, logType) {
		return nil
	}
	return fmt.Errorf("%s: unknown log type %q (want one of %v)", fileType, logType, valid)
}
