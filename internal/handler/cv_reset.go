package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

// ResetCVFromResume rebuilds a tailored CV's content from the current résumé seed
// (experience bank + structured extract — the same source first-time tailor uses), keeps
// the same CV id and agent session, and refreshes the base CV from that same seed so ATS
// delta and future bootstraps stay aligned. Cookie-only: destructive whole-document replace.
//
// Upload does not do this. Upload refreshes the seed source; this is the explicit apply.
func (h *cvHandlers) ResetCVFromResume(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := cvPathID(c)
	if err != nil {
		return err
	}

	// Ownership before seed: a foreign id must 404 even when the caller has no résumé
	// (otherwise "add a résumé" would leak that the CV exists).
	rec, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	if !rec.IsTailored {
		return fiber.NewError(fiber.StatusConflict, "reset from résumé is only for a tailored CV")
	}

	st, ok, err := h.seedSource().Structured(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusConflict, "add a résumé before resetting from it")
	}
	seeded := cv.Seed(st)

	if err := h.reseedBaseFromSeed(c, userID, seeded); err != nil {
		return err
	}

	_, _, err = h.editor.CommitDocument(c.Context(), id, userID,
		cvedit.ActorCandidate, cvedit.OriginImport,
		applySeedContent(cvedit.State{
			Title:      rec.Title,
			TemplateID: rec.TemplateID,
			Document:   rec.Document,
		}, seeded))
	if err != nil {
		return mapCVError(err)
	}

	out, err := h.cvStore.Get(c.Context(), id, userID)
	if err != nil {
		return mapCVError(err)
	}
	return c.JSON(fiber.Map{"data": recordResponse(out)})
}

// reseedBaseFromSeed refreshes the current base CV from seeded content, or creates one when
// the user has none. Presentation on an existing base is preserved.
func (h *cvHandlers) reseedBaseFromSeed(c *fiber.Ctx, userID int64, seeded cv.Document) error {
	base, ok, err := h.cvStore.BaseCV(c.Context(), userID)
	if err != nil {
		return err
	}
	if !ok {
		meta, err := h.cvStore.Create(c.Context(), userID, "My CV", cv.DefaultTemplateID, seeded)
		if err != nil {
			return err
		}
		// Best-effort history milestone — same pattern as TailorCV opening a tailored copy.
		if _, err := h.editor.Seed(c.Context(), meta.ID, userID, "Created from your résumé"); err != nil {
			log.Printf("cv: seeding the revision history for base %s: %v", meta.ID, err)
		}
		return nil
	}
	_, _, err = h.editor.CommitDocument(c.Context(), base.ID, userID,
		cvedit.ActorCandidate, cvedit.OriginImport,
		applySeedContent(cvedit.State{
			Title:      base.Title,
			TemplateID: base.TemplateID,
			Document:   base.Document,
		}, seeded))
	return mapCVError(err)
}

// reseedBaseIfStaleVsUpload refreshes the base from bankedSeeder when it predates the
// caller's current résumé upload. No-op when there is no base, no upload stamp, the base
// was edited at/after the upload, or the seed is unusable.
func (h *cvHandlers) reseedBaseIfStaleVsUpload(c *fiber.Ctx, userID int64) error {
	if h.resume == nil {
		return nil
	}
	uploadedAt, err := h.resume.UploadedAt(c.Context(), userID)
	if err != nil || uploadedAt == nil {
		return err
	}
	base, ok, err := h.cvStore.BaseCV(c.Context(), userID)
	if err != nil || !ok {
		return err
	}
	if !base.UpdatedAt.Before(*uploadedAt) {
		return nil
	}
	st, usable, err := h.seedSource().Structured(c.Context(), userID)
	if err != nil || !usable {
		return err
	}
	return h.reseedBaseFromSeed(c, userID, cv.Seed(st))
}
