package scanner

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gnana997/uispec/pkg/catalog"
	"github.com/gnana997/uispec/pkg/extractor"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/parser/queries"
)

// Scanner orchestrates the scan pipeline (Phases 1-3).
type Scanner struct {
	pm  *parser.ParserManager
	qm  *queries.QueryManager
	ext *extractor.Extractor
	log *slog.Logger
}

// NewScanner creates a scanner with all required dependencies.
func NewScanner(logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	pm := parser.NewParserManager(logger)
	qm := queries.NewQueryManager(pm, logger)
	ext := extractor.NewExtractor(pm, qm, logger)
	return &Scanner{pm: pm, qm: qm, ext: ext, log: logger}
}

// Run executes Phases 1-3 and returns the scan result.
func (s *Scanner) Run(rootDir string, cfg ScanConfig) (*ScanResult, error) {
	totalStart := time.Now()
	stats := ScanStats{}

	// Phase 1: File Discovery
	discoveryStart := time.Now()
	files, err := DiscoverFiles(rootDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}
	stats.FilesDiscovered = len(files)
	stats.DiscoveryTimeMs = time.Since(discoveryStart).Milliseconds()

	s.log.Info("discovery complete", "files", len(files), "ms", stats.DiscoveryTimeMs)

	if len(files) == 0 {
		stats.TotalTimeMs = time.Since(totalStart).Milliseconds()
		return &ScanResult{Stats: stats}, nil
	}

	// Phase 2: Extraction
	extractionStart := time.Now()
	results, failed := ExtractAll(files, s.ext, s.log)
	stats.FilesExtracted = len(results)
	stats.FilesFailed = failed
	stats.ExtractionTimeMs = time.Since(extractionStart).Milliseconds()

	s.log.Info("extraction complete",
		"extracted", len(results), "failed", failed, "ms", stats.ExtractionTimeMs)

	// Phase 3: Component Detection
	detectionStart := time.Now()
	components, groups := DetectComponents(results, s.pm)
	stats.ComponentsDetected = len(components)
	stats.CompoundGroups = len(groups)
	stats.DetectionTimeMs = time.Since(detectionStart).Milliseconds()

	s.log.Info("detection complete",
		"components", len(components), "groups", len(groups), "ms", stats.DetectionTimeMs)

	stats.TotalTimeMs = time.Since(totalStart).Milliseconds()

	return &ScanResult{
		Components:     components,
		CompoundGroups: groups,
		Stats:          stats,
	}, nil
}

// RunFull executes all phases (1-3 → 4 → 6) and returns a complete catalog.
func (s *Scanner) RunFull(rootDir string, cfg ScanConfig, buildCfg CatalogBuildConfig) (*catalog.Catalog, *ScanStats, error) {
	totalStart := time.Now()
	stats := ScanStats{}

	// Phase 1: File Discovery
	discoveryStart := time.Now()
	files, err := DiscoverFiles(rootDir, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("discovery failed: %w", err)
	}
	stats.FilesDiscovered = len(files)
	stats.DiscoveryTimeMs = time.Since(discoveryStart).Milliseconds()

	s.log.Info("discovery complete", "files", len(files), "ms", stats.DiscoveryTimeMs)

	if len(files) == 0 {
		stats.TotalTimeMs = time.Since(totalStart).Milliseconds()
		return nil, &stats, fmt.Errorf("no component files found in %s", rootDir)
	}

	// Phase 2: Extraction
	extractionStart := time.Now()
	results, failed := ExtractAll(files, s.ext, s.log)
	stats.FilesExtracted = len(results)
	stats.FilesFailed = failed
	stats.ExtractionTimeMs = time.Since(extractionStart).Milliseconds()

	s.log.Info("extraction complete",
		"extracted", len(results), "failed", failed, "ms", stats.ExtractionTimeMs)

	// Phase 3: Component Detection
	detectionStart := time.Now()
	components, groups := DetectComponents(results, s.pm)
	stats.ComponentsDetected = len(components)
	stats.CompoundGroups = len(groups)
	stats.DetectionTimeMs = time.Since(detectionStart).Milliseconds()

	s.log.Info("detection complete",
		"components", len(components), "groups", len(groups), "ms", stats.DetectionTimeMs)

	// Build results-by-file map for Phase 4.
	resultsByFile := make(map[string]*FileExtractionResult, len(results))
	for i := range results {
		resultsByFile[results[i].FilePath] = &results[i]
	}

	// Phase 4: Prop Extraction
	propStart := time.Now()
	propsMap := ExtractAllProps(components, resultsByFile, s.pm)
	stats.PropExtractionTimeMs = time.Since(propStart).Milliseconds()

	totalProps := 0
	for _, pr := range propsMap {
		totalProps += len(pr.Props)
	}
	stats.PropsExtracted = totalProps

	s.log.Info("prop extraction complete",
		"props", totalProps, "ms", stats.PropExtractionTimeMs)

	// Phase 5a: Node.js Enrichment (optional)
	if tsconfig, runtime, ok := CanEnrich(rootDir, s.log); ok {
		// Collect unique file paths from detected components.
		fileSet := make(map[string]struct{})
		for _, comp := range components {
			fileSet[comp.FilePath] = struct{}{}
		}
		enrichFiles := make([]string, 0, len(fileSet))
		for f := range fileSet {
			enrichFiles = append(enrichFiles, f)
		}

		enrichResult, err := RunEnrich(EnrichConfig{
			RootDir: rootDir,
			Files:   enrichFiles,
		}, runtime, tsconfig, s.log)
		if err != nil {
			s.log.Warn("enrichment failed, continuing with tree-sitter data only", "error", err)
		} else {
			MergeEnrichedProps(propsMap, enrichResult, components)
			stats.EnrichedComponents = len(enrichResult.Components)
			stats.EnrichmentTimeMs = enrichResult.DurationMs

			// Recount total props after enrichment (may have added inherited props).
			totalProps = 0
			for _, pr := range propsMap {
				totalProps += len(pr.Props)
			}
			stats.PropsExtracted = totalProps

			s.log.Info("enrichment merged",
				"enriched_components", stats.EnrichedComponents,
				"total_props", totalProps,
				"ms", stats.EnrichmentTimeMs)
		}
	} else {
		s.log.Info("enrichment skipped (no node/bun, tsconfig, or node_modules)")
	}

	// Phase 6: Catalog Build
	buildStart := time.Now()
	scanResult := &ScanResult{
		Components:     components,
		CompoundGroups: groups,
		Stats:          stats,
	}

	cat, err := BuildCatalog(scanResult, propsMap, buildCfg)
	stats.CatalogBuildTimeMs = time.Since(buildStart).Milliseconds()
	stats.TotalTimeMs = time.Since(totalStart).Milliseconds()

	s.log.Info("catalog build complete",
		"catalog_components", len(cat.Components), "ms", stats.CatalogBuildTimeMs)

	return cat, &stats, err
}

// Close releases parser and query manager resources.
func (s *Scanner) Close() {
	s.qm.Close()
	s.pm.Close()
}
