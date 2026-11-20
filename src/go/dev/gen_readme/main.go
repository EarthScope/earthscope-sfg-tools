// Command gen_readme rewrites the mermaid diagram in pkg/README.md from the
// log sources declared in package datamodels. It first runs
// datamodels.CheckDefinitions and fileparse.CheckRegistry. With -check it changes nothing and exits 1 if
// the diagram is out of date. Run it from src/go.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/datamodels"
	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/fileparse"
)

const (
	readme = "pkg/README.md"
	open   = "```mermaid\n"
	closer = "```"
)

func main() {
	check := flag.Bool("check", false, "exit 1 if the README diagram is out of date, without writing")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "gen_readme:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	if err := datamodels.CheckDefinitions(); err != nil {
		return err
	}
	if err := fileparse.CheckRegistry(); err != nil {
		return err
	}
	data, err := os.ReadFile(readme)
	if err != nil {
		return err
	}
	text := string(data)
	start := strings.Index(text, open)
	if start < 0 {
		return fmt.Errorf("%s: no mermaid block", readme)
	}
	start += len(open)
	end := strings.Index(text[start:], closer)
	if end < 0 {
		return fmt.Errorf("%s: unterminated mermaid block", readme)
	}
	end += start
	updated := text[:start] + datamodels.SourcesMermaid() + text[end:]
	if updated == text {
		return nil
	}
	if check {
		return fmt.Errorf("%s: diagram out of date; run go run ./dev/gen_readme", readme)
	}
	return os.WriteFile(readme, []byte(updated), 0o644)
}
