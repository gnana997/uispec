package main

import (
	"github.com/charmbracelet/huh/spinner"

	"github.com/gnana997/uispec/pkg/catalog"
	pkgscanner "github.com/gnana997/uispec/pkg/scanner"
)

// scanWithSpinner wraps the scan operation with an animated spinner on TTY.
// On non-TTY or quiet mode, uses the standard progress reporter.
func scanWithSpinner(
	s *pkgscanner.Scanner,
	directory string,
	scanCfg pkgscanner.ScanConfig,
	buildCfg pkgscanner.CatalogBuildConfig,
) (*catalog.Catalog, *pkgscanner.ScanStats, error) {
	// Non-TTY or quiet: use standard progress or none.
	if !outCfg.IsTTY || outCfg.Verbosity == VerbQuiet {
		if outCfg.Verbosity != VerbQuiet {
			s.SetProgress(pkgscanner.NewProgress(true))
		}
		return s.RunFull(directory, scanCfg, buildCfg)
	}

	// TTY mode: show spinner while scanning.
	var cat *catalog.Catalog
	var stats *pkgscanner.ScanStats
	var scanErr error

	err := spinner.New().
		Title(" Scanning components...").
		Action(func() {
			// Disable text progress since spinner handles the visual feedback.
			s.SetProgress(pkgscanner.NewProgress(false))
			cat, stats, scanErr = s.RunFull(directory, scanCfg, buildCfg)
		}).
		Run()

	if err != nil {
		return nil, nil, err
	}
	return cat, stats, scanErr
}
