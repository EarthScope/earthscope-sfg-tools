package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/earthscope/gnsstools/compress/crinex"
)

// runCrinexCompress and runCrinexDecompress are intentionally thin: they
// parse flags and delegate all I/O logic to the crinex package, which
// handles stdin/stdout ("-"), transparent gzip detection, and file open/close.
func runCrinexCompress(cmd *cobra.Command, args []string) error {
	inPath, _ := cmd.Flags().GetString("input")
	outPath, _ := cmd.Flags().GetString("output")
	gzipOut, _ := cmd.Flags().GetBool("gzip")
	if err := crinex.CrinexCompress(inPath, outPath, gzipOut); err != nil {
		return fmt.Errorf("compressing RINEX to CRINEX: %w", err)
	}
	return nil
}

func runCrinexDecompress(cmd *cobra.Command, args []string) error {
	inPath, _ := cmd.Flags().GetString("input")
	outPath, _ := cmd.Flags().GetString("output")
	if err := crinex.CrinexDecompress(inPath, outPath); err != nil {
		return fmt.Errorf("decompressing CRINEX to RINEX: %w", err)
	}
	return nil
}

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

	crinexCmd.AddCommand(crinexCompressCmd)
	crinexCmd.AddCommand(crinexDecompressCmd)
}
