//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

func buildResetApp(h *cvHandlers, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss, testVersions)
	keyAuth := auth.RequireAuthOrScopedKey(iss, testVersions, apiKeys{h.queries}, auth.ScopeCV)
	app.Get("/api/v1/me/cvs/:id", keyAuth, h.GetCV)
	app.Post("/api/v1/me/cvs/:id/reset-from-resume", saved, h.ResetCVFromResume)
	return app
}

type resetFixture struct {
	h        *cvHandlers
	app      *fiber.App
	iss      *auth.Issuer
	pool     *pgxpool.Pool
	token    string
	userID   int64
	base     cv.Meta
	tailored cv.Meta
	store    *cv.Store
}

func newResetFixture(t *testing.T) resetFixture {
	t.Helper()
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "reset@example.com", false)
	tok, err := iss.Issue(userID, 1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Summary:  "Résumé seed summary",
		Skills:   []string{"Go", "PostgreSQL"},
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now(),
		 resume_structured_model = 'test' WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}

	store := h.cvStore
	base, err := store.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Margins: cv.Margins{Top: 0.75, Right: 0.75, Bottom: 0.75, Left: 0.75},
		Style:   cv.Style{FontSize: 11},
		Header:  cv.Header{FullName: "Old Base"},
		Summary: "old base summary",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	jobID := seedJobSlug(t, pool, "reset-job-"+uuid.NewString()[:8])
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored for Role", cv.DefaultTemplateID, cv.Document{
		Margins: cv.Margins{Top: 0.6, Right: 0.6, Bottom: 0.6, Left: 0.6},
		Style:   cv.Style{FontSize: 10.5, LineHeight: 0.5},
		Header:  cv.Header{FullName: "Old Tailored"},
		Summary: "old tailored summary",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	if err := store.SetSession(ctx, tailored.ID, userID, "sess-keep"); err != nil {
		t.Fatalf("set session: %v", err)
	}

	return resetFixture{
		h: h, app: buildResetApp(h, iss), iss: iss, pool: pool, token: tok,
		userID: userID, base: base, tailored: tailored, store: store,
	}
}

func TestResetCVFromResume_HappyPath(t *testing.T) {
	f := newResetFixture(t)
	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, f.token, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.ID != f.tailored.ID.String() {
		t.Fatalf("id = %s, want same tailored id %s", out.Data.ID, f.tailored.ID)
	}
	if out.Data.Document.Header.FullName != "Ada Lovelace" || out.Data.Document.Summary != "Résumé seed summary" {
		t.Fatalf("tailored content = %+v / %q, want résumé seed", out.Data.Document.Header, out.Data.Document.Summary)
	}
	if out.Data.Document.Margins.Top != 0.6 || out.Data.Document.Style.FontSize != 10.5 {
		t.Fatalf("tailored presentation clobbered: margins=%+v style=%+v", out.Data.Document.Margins, out.Data.Document.Style)
	}
	if out.Data.AgentSessionID != "sess-keep" {
		t.Fatalf("session = %q, want sess-keep", out.Data.AgentSessionID)
	}

	base, ok, err := f.store.BaseCV(context.Background(), f.userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if base.ID != f.base.ID {
		t.Fatalf("base id changed: %s → %s", f.base.ID, base.ID)
	}
	if base.Document.Header.FullName != "Ada Lovelace" || base.Document.Summary != "Résumé seed summary" {
		t.Fatalf("base content = %+v / %q", base.Document.Header, base.Document.Summary)
	}
	if base.Document.Margins.Top != 0.75 || base.Document.Style.FontSize != 11 {
		t.Fatalf("base presentation clobbered: margins=%+v style=%+v", base.Document.Margins, base.Document.Style)
	}
}

func TestResetCVFromResume_CreatesBaseWhenAbsent(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "nobase@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "New Ada", Summary: "from seed"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now() WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}
	jobID := seedJobSlug(t, pool, "nobase-"+uuid.NewString()[:8])
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "Only Tailored", cv.DefaultTemplateID, cv.Document{
		Summary: "stale",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	base, ok, err := h.cvStore.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("expected a base CV after reset: ok=%v err=%v", ok, err)
	}
	if base.Document.Header.FullName != "New Ada" {
		t.Fatalf("new base FullName = %q", base.Document.Header.FullName)
	}
}

func TestResetCVFromResume_BaseTarget409(t *testing.T) {
	f := newResetFixture(t)
	path := "/api/v1/me/cvs/" + f.base.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, f.token, nil)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestResetCVFromResume_NoSeed409(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "noseed@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	jobID := seedJobSlug(t, pool, "noseed-"+uuid.NewString()[:8])
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "T", cv.DefaultTemplateID, cv.Document{Summary: "x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 409", resp.StatusCode, body)
	}
}

func TestResetCVFromResume_OtherOwner404(t *testing.T) {
	f := newResetFixture(t)
	otherID := seedAccount(t, f.pool, "other-reset@example.com", false)
	otherTok, err := f.iss.Issue(otherID, 1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, otherTok, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestResetCVFromResume_DoesNotChangeProfile(t *testing.T) {
	f := newResetFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO user_profiles (user_id, specializations, skills, excluded_skills)
		 VALUES ($1, ARRAY['backend'], ARRAY['go'], ARRAY[]::text[])`,
		f.userID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, f.token, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var skills []string
	if err := f.pool.QueryRow(ctx,
		`SELECT skills FROM user_profiles WHERE user_id = $1`, f.userID).Scan(&skills); err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if len(skills) != 1 || skills[0] != "go" {
		t.Fatalf("profile skills = %v, want [go] unchanged (Reset must not merge)", skills)
	}
}

// A bank role with more banked, publishable claims than cv.MaxBullets is exactly the seed
// CommitDocument used to truncate silently (it sanitizes before diffing, so the refuse
// guard never saw the overflow). Reset must refuse instead — and because the tailored
// target now commits before the base refresh, a refusal must leave BOTH untouched rather
// than a base already rewritten under a request that reports failure.
func TestResetCVFromResume_RefusesWhenTheBankSeedExceedsTheBulletCap(t *testing.T) {
	prevMax := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prevMax) })

	h, iss, pool := newTailorAPI(t)
	bank := experience.NewStore(experience.NewQueriesRepository(h.queries))
	h.seeder = bankedSeeder{resume: h.resume, bank: bank}

	userID := seedAccount(t, pool, "overcap-reset@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()

	emp, err := bank.CreateEmployment(ctx, userID, experience.Employment{
		Kind: experience.KindJob, Company: "Neon", Role: "Staff Engineer",
		Start: "2018", End: "2024",
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	for i := 0; i < cv.MaxBullets+1; i++ {
		if _, err := bank.AddAtom(ctx, userID, experience.Atom{
			EmploymentID: &emp.ID,
			Claim:        fmt.Sprintf("Banked achievement %d", i+1),
			Provenance:   experience.ProvenanceStatedInChat,
		}); err != nil {
			t.Fatalf("AddAtom %d: %v", i, err)
		}
	}

	store := h.cvStore
	base, err := store.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Old Base"}, Summary: "old base summary",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	jobID := seedJobSlug(t, pool, "overcap-reset-job")
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored for Role", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Old Tailored"}, Summary: "old tailored summary",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 409: %s", resp.StatusCode, body)
	}

	gotTailored, err := store.Get(ctx, tailored.ID, userID)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if gotTailored.Document.Summary != "old tailored summary" {
		t.Fatalf("tailored summary = %q, want untouched by a refused reset", gotTailored.Document.Summary)
	}
	gotBase, ok, err := store.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if gotBase.ID != base.ID || gotBase.Document.Summary != "old base summary" {
		t.Fatalf("base = %+v, want untouched — the target failed before the base refresh ran", gotBase.Document)
	}
}
