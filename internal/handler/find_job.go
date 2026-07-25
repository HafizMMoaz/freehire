package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/sources"
)

// FindJob resolves the URL of a job posting in the wild to a freehire catalog
// slug, so the browser extension can tell that the page it is on is a job we
// already carry and switch from the ad-hoc text match to the curated card.
// Public; returns {"data": null} whenever the posting cannot be identified.
//
// Two tiers, exact first. The catalog's own dedup identity — (source,
// external_id), recovered from the URL — is served by a unique index, but only a
// handful of ATS URL shapes carry an identity a parser can read. So a URL that
// names no identity, or names one we have no row for, falls through to matching
// the page against the detail URL each source stored in jobs.url (normalized on
// both sides, see migration 0042): for aggregators and most boards that IS the
// page the user is standing on.
//
// Neither tier guesses. An earlier version matched the page title against every
// catalog title, which could not use an index at all (the LIKE pattern was built
// from the column), so it degenerated into a sequential scan of millions of rows
// and timed out in production — and it was guesswork besides, on a page title
// that is not something to guess from (the extension was sending "reCAPTCHA").
func (a *API) FindJob(c *fiber.Ctx) error {
	pageURL := strings.TrimSpace(c.Query("url"))
	// Nothing to resolve — and an empty URL must not reach the second tier, where it
	// would normalize to "" and match a posting stored with an empty url.
	if pageURL == "" {
		return c.JSON(fiber.Map{"data": nil})
	}

	if ref, ok := sources.RefFromURL(pageURL); ok {
		job, err := a.queries.GetJobBySourceExternalID(c.Context(), db.GetJobBySourceExternalIDParams{
			Source:     ref.Source,
			ExternalID: ref.ExternalID,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && job.PublicSlug != "" {
			return c.JSON(fiber.Map{"data": fiber.Map{"public_slug": job.PublicSlug}})
		}
	}

	slug, err := a.queries.FindOpenJobByURL(c.Context(), pageURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(fiber.Map{"data": nil})
		}
		return err
	}
	if slug == "" {
		return c.JSON(fiber.Map{"data": nil})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"public_slug": slug}})
}
