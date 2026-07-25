package sources

import (
	"net/url"
	"regexp"
	"strings"
)

// PostingRef is a catalog identity — the (source, external_id) key jobs are
// deduplicated by — recovered from the URL of a job posting in the wild.
//
// It exists so a client that is *looking at* a posting can ask which catalog job
// it is, exactly, instead of describing it and hoping: matching a page title
// against catalog titles is both unreliable (an ATS page title is full of noise)
// and expensive (no index can serve it).
type PostingRef struct {
	Source     string
	ExternalID string
}

// greenhouseJobPath matches the job-detail form, /<board>/jobs/<id>.
var greenhouseJobPath = regexp.MustCompile(`^/([^/]+)/jobs/(\d+)/?$`)

// RefFromURL recovers the catalog identity from a posting URL, reporting false
// when the URL is not one we can resolve — an unknown ATS, or a link that
// carries the job id but not the board it belongs to (a company's own careers
// page, say). A false answer is the honest one: the caller should then leave the
// posting unrecognised rather than guess.
//
// Only Greenhouse is understood today. Each ATS numbers and addresses its
// postings differently, and the board segment of the URL has to be the same
// token the ingest board file uses, so support is added per provider against
// real links rather than inferred from a pattern.
func RefFromURL(raw string) (PostingRef, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return PostingRef{}, false
	}

	switch strings.ToLower(u.Hostname()) {
	case "job-boards.greenhouse.io", "boards.greenhouse.io":
		return greenhouseRef(u)
	default:
		return PostingRef{}, false
	}
}

// greenhouseRef reads a Greenhouse link in either of its two shapes: the
// embedded application form (/embed/job_app?for=<board>&token=<id>) and the job
// detail page (/<board>/jobs/<id>). The form's `jr_id` is Greenhouse's internal
// requisition id, which the catalog does not store — the job id is `token`.
func greenhouseRef(u *url.URL) (PostingRef, bool) {
	if strings.HasPrefix(u.Path, "/embed/job_app") {
		board := u.Query().Get("for")
		id := u.Query().Get("token")
		return greenhousePosting(board, id)
	}
	if m := greenhouseJobPath.FindStringSubmatch(u.Path); m != nil {
		return greenhousePosting(m[1], m[2])
	}
	return PostingRef{}, false
}

func greenhousePosting(board, jobID string) (PostingRef, bool) {
	board = strings.ToLower(strings.TrimSpace(board))
	jobID = strings.TrimSpace(jobID)
	if board == "" || jobID == "" || !isDigits(jobID) {
		return PostingRef{}, false
	}
	return PostingRef{Source: "greenhouse", ExternalID: NamespaceExternalID(board, jobID)}, true
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
