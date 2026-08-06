package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// searchIndexer adapts the search client to searchdrain.Indexer: build each job's
// document from its persisted row (search.FromJob) and push the whole wave into the
// live facet index as one Meili task. It also attaches the job-reality signal and
// widens the canon's geography with its role cluster's — the same enrichment the old
// inline ingest push and cmd/embed's semantic indexer both attach — so a job first
// indexed here carries them immediately rather than only after the next full reindex.
type searchIndexer struct {
	client *search.Client
	q      *db.Queries
}

func (ix searchIndexer) IndexBatch(ctx context.Context, jobs []db.Job) error {
	docs := make([]search.JobDocument, 0, len(jobs))
	for _, job := range jobs {
		// A job whose category neither the title dictionary nor the LLM ever resolved
		// (search.CategoryUnresolved) never enters the index — see cmd/reindex's splitJobs
		// for the same rule applied to the full-rebuild path. This skips rather than
		// deletes: if the row is a rare pre-existing index entry from before this rule
		// existed, it goes stale here the same way a closed job's does between drain
		// waves (internal/searchdrain/AGENTS.md) — the next full reindex swap is the
		// reconciler for both.
		if search.CategoryUnresolved(job) {
			continue
		}
		doc, err := search.FromJob(job)
		if err != nil {
			return fmt.Errorf("build document (job %d): %w", job.ID, err)
		}
		repost, mass := int64(1), int64(1)
		// askGeo defaults to true because a FAILED count must not also suppress the
		// geography merge below: skipping it is destructive (the push replaces the
		// stored union), not conservative. Only a known singleton can safely skip.
		askGeo := true
		if c, err := ix.q.RoleClusterCount(ctx, db.RoleClusterCountParams{
			CompanySlug:     job.CompanySlug,
			RoleFingerprint: job.RoleFingerprint,
		}); err != nil {
			log.Printf("search-drain: role-cluster count for job %d: %v", job.ID, err)
		} else {
			repost, mass = c.RepostCount, c.MassCount
			askGeo = mass > 1
		}
		reality := jobview.ClassifyReality(job, time.Now(), int(repost), int(mass))
		doc.Reality = &reality
		if askGeo {
			if g, err := ix.q.RoleClusterGeo(ctx, db.RoleClusterGeoParams{
				CompanySlug:     job.CompanySlug,
				RoleFingerprint: job.RoleFingerprint,
			}); err != nil {
				log.Printf("search-drain: role-cluster geography for job %d: %v", job.ID, err)
			} else {
				doc.MergeClusterGeography(g.Countries, g.Regions, g.Cities)
			}
		}
		docs = append(docs, doc)
	}
	return ix.client.IndexJobs(ctx, docs)
}
