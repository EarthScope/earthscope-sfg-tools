package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sfg",
	Short: "EarthScope seafloor geodesy tools",
	Long:  "A collection of tools for processing NovAtel GNSS data into RINEX and TileDB formats.",
}

func init() {
	rootCmd.AddCommand(crinexCmd)
	rootCmd.AddCommand(nova2rnxCmd)
	rootCmd.AddCommand(novb2rnxCmd)
	rootCmd.AddCommand(nov0002rnxCmd)
	rootCmd.AddCommand(nova2tileCmd)
	rootCmd.AddCommand(novab2tileCmd)
	rootCmd.AddCommand(nov0002tileCmd)
	rootCmd.AddCommand(tdb2rnxCmd)
	rootCmd.AddCommand(rnxqcCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
