package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// remotli adapts remotli.ch, a Swiss remote-jobs aggregator. Boardless (one public API, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's company from the feed. The /api/jobs endpoint pages by page number over a
// reported totalPages, and ships each posting's full description inline (no detail call).
//
// Note: remotli.ch's robots.txt disallows /api/ for every listed user agent. The endpoint is
// public and unauthenticated, and the crawl is a scheduled job rather than a search-engine
// crawler, but this is flagged here because it was flagged in the adapter's tracking issue as
// a conscious call rather than a silent one.
type remotli struct {
	http JSONGetter
}

const remotliListURL = "https://remotli.ch/api/jobs?page=%d"

// NewRemotli builds the Remotli adapter over the given HTTP client.
func NewRemotli(c JSONGetter) Source { return remotli{http: c} }

func (remotli) Provider() string { return "remotli" }

func (remotli) boardless() {}

func (remotli) aggregator() {}

// remotliResponse is one page: postings plus pagination info used to decide whether another
// page is due. Each element of Jobs is wrapped one level deep under its own "jobs" key.
type remotliResponse struct {
	Jobs []struct {
		Job remotliPosting `json:"jobs"`
	} `json:"jobs"`
	Pagination struct {
		Page       int `json:"page"`
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
}

// remotliPosting is one posting, body inline (no detail call). Status gives liveness at
// source, so only "active" postings are emitted.
type remotliPosting struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	Type        string `json:"type"`
	Description string `json:"description"`
	ApplyURL    string `json:"applyUrl"`
	Status      string `json:"status"`
	PublishedAt string `json:"publishedAt"`
}

func (s remotli) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		var resp remotliResponse
		url := fmt.Sprintf(remotliListURL, page)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			// Only a failure on the very first page is a board-level error; a later page
			// failing ends the walk with what was gathered so far (see internal/sources/AGENTS.md).
			if page == 1 {
				return nil, fmt.Errorf("remotli: list page %d: %w", page, err)
			}
			return jobs, nil
		}
		for _, wrapper := range resp.Jobs {
			if job, ok := wrapper.Job.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if page >= resp.Pagination.TotalPages {
			break
		}
	}
	return jobs, nil
}

// toJob maps an inline posting to a Job, returning ok=false for an unusable posting (no
// native id, no company which would break the slug, or a status other than "active" — the
// board reports liveness at source, so a non-active posting is dropped rather than upserted).
func (p remotliPosting) toJob() (Job, bool) {
	if p.ID == 0 || p.Company == "" || p.Status != "active" {
		return Job{}, false
	}
	return Job{
		ExternalID: strconv.FormatInt(p.ID, 10),
		URL:        p.ApplyURL,
		Title:      p.Title,
		Company:    p.Company,
		Location:   p.Location,
		// Remote is the shared free-text heuristic: unlike weworkremotely/nodesk, this board
		// mixes onsite Swiss postings (e.g. "Zürich, Switzerland") in with remote ones, and the
		// API carries no structured remote flag (remotePolicy is always null in practice), so
		// WorkMode is left for the pipeline's location dictionary to derive.
		Remote:         isRemote(p.Location),
		Description:    sanitizeHTML(p.Description),
		EmploymentType: schemaEmploymentType(strings.ReplaceAll(p.Type, "-", "_")),
		PostedAt:       parseRFC3339(p.PublishedAt),
	}, true
}
