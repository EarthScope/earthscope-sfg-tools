package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/internal/crinex"
	"gitlab.com/earthscope/gnsstools/products"
)

var crinexCmd = &cobra.Command{
	Use:   "crinex",
	Short: "Compress or decompress RINEX with Hatanaka (CRINEX)",
	Long: `Compress RINEX observation files to CRINEX format (Hatanaka)
or decompress CRINEX files back to RINEX.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var crinexCompressCmd = &cobra.Command{
	Use:   "compress",
	Short: "Compress RINEX observation text to CRINEX",
	RunE:  runCrinexCompress,
}

var crinexDecompressCmd = &cobra.Command{
	Use:   "decompress",
	Short: "Decompress CRINEX to RINEX observation text",
	RunE:  runCrinexDecompress,
}

func init() {
	crinexCompressCmd.Flags().String("input", "-", "Input path (use - for stdin, .gz auto-detected)")
	crinexCompressCmd.Flags().String("output", "-", "Output path (use - for stdout)")
	crinexCompressCmd.Flags().BoolP("gzip", "z", false, "Gzip-compress the output")

	crinexDecompressCmd.Flags().String("input", "-", "Input path (use - for stdin, .gz auto-detected)")
	crinexDecompressCmd.Flags().String("output", "-", "Output path (use - for stdout)")
	crinexDecompressCmd.Flags().BoolP("gzip", "z", false, "Gzip-compress the output")

	crinexCmd.AddCommand(crinexCompressCmd)
	crinexCmd.AddCommand(crinexDecompressCmd)
}

func runCrinexCompress(cmd *cobra.Command, args []string) error {
	inPath, _ := cmd.Flags().GetString("input")
	outPath, _ := cmd.Flags().GetString("output")
	gzipOut, _ := cmd.Flags().GetBool("gzip")

	r, err := openInput(inPath)
	if err != nil {
		return fmt.Errorf("opening input: %w", err)
	}
	defer r.Close()

	w, err := openOutput(outPath)
	if err != nil {
		return fmt.Errorf("opening output: %w", err)
	}
	defer w.Close()

	var out io.Writer = w
	if gzipOut {
		gz := gzip.NewWriter(w)
		defer gz.Close()
		out = gz
	}

	if err := crinex.RNX2CRNX(r, out); err != nil {
		return fmt.Errorf("compressing RINEX to CRINEX: %w", err)
	}
	return nil
}

func runCrinexDecompress(cmd *cobra.Command, args []string) error {
	inPath, _ := cmd.Flags().GetString("input")
	outPath, _ := cmd.Flags().GetString("output")
	gzipOut, _ := cmd.Flags().GetBool("gzip")

	r, err := openInput(inPath)
	if err != nil {
		return fmt.Errorf("opening input: %w", err)
	}
	defer r.Close()

	w, err := openOutput(outPath)
	if err != nil {
		return fmt.Errorf("opening output: %w", err)
	}
	defer w.Close()

	var out io.Writer = w
	if gzipOut {
		gz := gzip.NewWriter(w)
		defer gz.Close()
		out = gz
	}

	if err := crinex.CRNX2RNX(r, out); err != nil {
		return fmt.Errorf("decompressing CRINEX to RINEX: %w", err)
	}
	return nil
}

// openInput opens a file for reading, transparently decompressing .gz files.
// The special path "-" returns stdin.
func openInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return products.OpenMaybeGzip(path)
}

func openOutput(path string) (io.WriteCloser, error) {
	if path == "-" {
		return nopWriteCloser{Writer: os.Stdout}, nil
	}
	return os.Create(path)
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func main() {
	if err := crinexCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}