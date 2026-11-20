package datamodels

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/edid"
)

// SourcesMermaid renders the Level 0 flow as a mermaid flowchart: each file
// type, the tables its log sources feed, and the log sources of each table.
// It is generated from the Sources of GNSSA_L0's tables, in their order, so
// the README diagram cannot drift from what the parsers stamp.
func SourcesMermaid() string {
	var fileTypes []string
	tableOf := map[string][]string{} // file type -> tables, in order
	for _, t := range GNSSA_L0.Tables {
		for _, s := range t.Sources {
			if !slices.Contains(fileTypes, s.FileType) {
				fileTypes = append(fileTypes, s.FileType)
			}
			if !slices.Contains(tableOf[s.FileType], t.Name) {
				tableOf[s.FileType] = append(tableOf[s.FileType], t.Name)
			}
		}
	}
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("flowchart LR")
	line("    %%%% File types")
	for _, ft := range fileTypes {
		line("    %s[%s]", ft, ft)
	}
	line("")
	line("    %%%% Level 0 tables")
	for _, t := range GNSSA_L0.Tables {
		line("    %s[(%s)]", t.Name, t.Name)
	}
	line("")
	line("    %%%% File type -> table")
	for _, ft := range fileTypes {
		for _, t := range tableOf[ft] {
			line("    %s --> %s", ft, t)
		}
	}
	line("")
	line("    %%%% Table -> log source (labelled file type · log source)")
	for _, t := range GNSSA_L0.Tables {
		for _, s := range t.Sources {
			line("    %s --> %s([%s · %s])", t.Name, nodeID(s), s.FileType, s.Name)
		}
	}
	return b.String()
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9_]`)

// nodeID is a log source's mermaid node ID, unique per (file type, name).
func nodeID(s edid.LogSource) string {
	return nonWord.ReplaceAllString(s.FileType+"_"+s.Name, "_")
}

// CheckDefinitions reports inconsistencies between the declarations: a ref
// naming a database that does not exist, a ref whose table is not in its
// database, SFG.Refs() and Refs differing, or a table listing the same log source twice.
func CheckDefinitions() error {
	var errs []string
	declared := map[string]bool{}
	for _, ref := range Refs {
		declared[ref.String()] = true
		db, ok := SFG.Database(ref.Database)
		if !ok {
			errs = append(errs, fmt.Sprintf("ref %s: no database %q", ref, ref.Database))
		} else if !db.Has(ref.Def) {
			errs = append(errs, fmt.Sprintf("ref %s: table not in database", ref))
		}
	}
	inCatalog := map[string]bool{}
	for _, ref := range SFG.Refs() {
		inCatalog[ref.String()] = true
		if !declared[ref.String()] {
			errs = append(errs, fmt.Sprintf("SFG table %s has no entry in Refs", ref))
		}
	}
	for _, ref := range Refs {
		if !inCatalog[ref.String()] {
			errs = append(errs, fmt.Sprintf("Refs entry %s is not in SFG", ref))
		}
	}
	for _, ref := range SFG.Refs() {
		seen := map[edid.LogSource]bool{}
		for _, s := range ref.Def.Sources {
			if seen[s] {
				errs = append(errs, fmt.Sprintf("table %s lists log source %s twice", ref, s))
			}
			seen[s] = true
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("definitions:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
