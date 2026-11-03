package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/EarthScope/es_sfgtools/src/golangtools/pkg/convert"
)

var rootCmd = &cobra.Command{
	Use:   "sfg",
	Short: "EarthScope seafloor geodesy tools",
	Long:  "A collection of tools for processing NovAtel GNSS data into RINEX and Level 1 tables.",
	// main prints errors, once.
	SilenceErrors: true,
}

// exitError ends the program with code; the command has already reported
// why.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func init() {
	rootCmd.AddCommand(crinexCmd)
	rootCmd.AddCommand(nova2rnxCmd)
	rootCmd.AddCommand(novb2rnxCmd)
	rootCmd.AddCommand(nov0002rnxCmd)
	rootCmd.AddCommand(nov2level1Cmd)
	rootCmd.AddCommand(level1Cmd)
	rootCmd.AddCommand(convertCmd)
	rootCmd.AddCommand(rnxqcCmd)
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &convert.UsageError{Err: err}
	})
}

func main() {
	err := rootCmd.Execute()
	var exit exitError
	var usage *convert.UsageError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		os.Exit(exit.code)
	case errors.As(err, &usage):
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(convert.ExitUsage)
	default:
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(convert.ExitFatal)
	}
}
