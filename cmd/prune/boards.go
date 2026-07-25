package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/sources"
)

// listedCompanies returns the company slugs still present in the board files, keyed by
// the same slug normalization the ingest write path applies — comparing anything else
// would silently match nothing.
//
// It is the gate on the company-scoped rules. Those have no ingest counterpart: nothing
// at crawl time knows a company's bucket, so a deletion under one is undone by the next
// hourly crawl unless the board is struck from the files in the same step. A company
// still listed here therefore cannot be pruned that way.
//
// It fails closed. An unreadable directory or a malformed board file is an error rather
// than a short listing, because a missing entry reads as "this board is retired" — and
// an empty listing would read as "every board is retired", clearing the guard on the
// entire catalogue.
func listedCompanies(dir string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, fmt.Errorf("prune: scan %s: %w", dir, err)
	}
	if len(paths) == 0 {
		// Distinguish "no board files" from "a directory of retired boards": the
		// former is a misconfigured path, and treating it as the latter would arm
		// every company-scoped rule at once.
		if _, statErr := os.Stat(dir); statErr != nil {
			return nil, fmt.Errorf("prune: source directory %s: %w", dir, statErr)
		}
		return nil, fmt.Errorf("prune: no board files under %s", dir)
	}

	listed := make(map[string]bool)
	for _, path := range paths {
		cfg, err := sources.LoadConfig(path)
		if err != nil {
			return nil, err
		}
		for _, e := range cfg.Sources {
			listed[normalize.Slug(e.Company)] = true
		}
	}
	return listed, nil
}
