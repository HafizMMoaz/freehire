package cvedit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/cv"
)

func TestCommitRefusesInsertThatWouldDropExistingBullets(t *testing.T) {
	prev := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prev) })

	repo := newFakeRepo()
	bullets := make([]string, cv.MaxBullets)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Original bullet %d — keep me", i+1)
	}
	repo.state.Experience = []cv.ExperienceItem{{
		Role: "Staff Engineer", Company: "Neon", Bullets: bullets,
	}}
	e, _ := newEditor(repo, &bank{})

	err := agentEdit(t, e, Op{
		Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[0]"),
		Value:      "Brand new keyword bullet",
		EvidenceID: "banked",
	})
	if !errors.Is(err, ErrListCap) {
		t.Fatalf("Commit = %v, want ErrListCap", err)
	}
	if !strings.Contains(err.Error(), ListCapCode) {
		t.Fatalf("error %q missing stable code %q for the UI", err, ListCapCode)
	}
	if !strings.Contains(err.Error(), "not applied") {
		t.Fatalf("error %q should tell the model nothing was written", err)
	}
	if repo.saves != 0 || len(repo.revisions) != 0 {
		t.Fatal("a refused over-cap insert must not save or file a revision")
	}
	if got := repo.state.Experience[0].Bullets[0]; got != bullets[0] {
		t.Fatalf("first bullet = %q, want the original kept", got)
	}
	if got := len(repo.state.Experience[0].Bullets); got != cv.MaxBullets {
		t.Fatalf("bullet count = %d, want still %d", got, cv.MaxBullets)
	}
}

func TestCommitRefusesOverCapInsertOnAProject(t *testing.T) {
	prev := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prev) })

	repo := newFakeRepo()
	bullets := make([]string, cv.MaxBullets)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Project bullet %d", i+1)
	}
	repo.state.Projects = []cv.Project{{Name: "freehire", Bullets: bullets}}
	e, _ := newEditor(repo, &bank{})

	err := agentEdit(t, e, Op{
		Kind: OpInsert, Path: mustParse(t, "projects[0].bullets[0]"),
		Value:      "Extra project claim",
		EvidenceID: "banked",
	})
	if !errors.Is(err, ErrListCap) {
		t.Fatalf("Commit = %v, want ErrListCap", err)
	}
	if !strings.Contains(err.Error(), "project freehire") {
		t.Fatalf("error %q should name the project", err)
	}
	if repo.saves != 0 || len(repo.revisions) != 0 {
		t.Fatal("a refused over-cap project insert must not save")
	}
	if got := repo.state.Projects[0].Bullets[0]; got != bullets[0] {
		t.Fatalf("first bullet = %q, want the original kept", got)
	}
}

func TestCommitRefusesOverCapForTheCandidateEditor(t *testing.T) {
	prev := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prev) })

	repo := newFakeRepo()
	bullets := make([]string, cv.MaxBullets)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Original bullet %d", i+1)
	}
	repo.state.Experience = []cv.ExperienceItem{{
		Role: "Eng", Company: "Acme", Bullets: bullets,
	}}
	e := NewEditor(repo, nil)

	_, _, err := e.Commit(context.Background(), uuid.Nil, 1, Change{
		Actor: ActorCandidate, Origin: OriginEditor,
		Ops: []Op{{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[0]"), Value: "One more"}},
	})
	if !errors.Is(err, ErrListCap) {
		t.Fatalf("Commit = %v, want ErrListCap", err)
	}
	if repo.saves != 0 {
		t.Fatal("candidate over-cap insert must not save")
	}
}

func TestCommitAllowsInsertWhenUnderTheCap(t *testing.T) {
	repo := newFakeRepo()
	repo.state.Experience = []cv.ExperienceItem{{
		Role: "Eng", Company: "Acme", Bullets: []string{"Only one"},
	}}
	e, _ := newEditor(repo, &bank{})

	if err := agentEdit(t, e, Op{
		Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[1]"),
		Value:      "Second bullet",
		EvidenceID: "banked",
	}); err != nil {
		t.Fatalf("Commit under the cap: %v", err)
	}
	if got := len(repo.state.Experience[0].Bullets); got != 2 {
		t.Fatalf("bullets = %d, want 2", got)
	}
}

func TestUserListCapMessageStripsModelRemedy(t *testing.T) {
	err := listCapErr("Staff Engineer at Neon")
	got := UserListCapMessage(err)
	if got == "" {
		t.Fatal("empty user message")
	}
	if strings.Contains(got, ListCapCode) || strings.Contains(got, "Set an existing") {
		t.Fatalf("user message still has internals: %q", got)
	}
	if !strings.Contains(got, "Your existing bullets were kept") {
		t.Fatalf("user message %q should reassure that nothing was deleted", got)
	}
	if !strings.Contains(got, "Staff Engineer at Neon") {
		t.Fatalf("user message %q should name the role", got)
	}
}

func TestCommitAllowsOverCapWhenRefuseIsDisabled(t *testing.T) {
	prev := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prev) })

	repo := newFakeRepo()
	bullets := make([]string, cv.MaxBullets)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Original bullet %d", i+1)
	}
	repo.state.Experience = []cv.ExperienceItem{{
		Role: "Staff Engineer", Company: "Neon", Bullets: bullets,
	}}
	e := NewEditor(repo, &bank{}).SetRefuseListCap(false)

	if err := agentEdit(t, e, Op{
		Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[0]"),
		Value:      "Brand new keyword bullet",
		EvidenceID: "banked",
	}); err != nil {
		t.Fatalf("Commit with refuse off: %v", err)
	}
	if got := len(repo.state.Experience[0].Bullets); got != cv.MaxBullets {
		t.Fatalf("bullets = %d, want Sanitize to keep %d when refuse is off", got, cv.MaxBullets)
	}
	if got := repo.state.Experience[0].Bullets[0]; got != "Brand new keyword bullet" {
		t.Fatalf("first bullet = %q, want the insert kept and the trailing original dropped", got)
	}
}

func TestCommitStillDropsWhitespaceOnlyBulletsWithoutRefusing(t *testing.T) {
	repo := newFakeRepo()
	repo.state.Experience = []cv.ExperienceItem{{
		Role: "Eng", Company: "Acme", Bullets: []string{"Keep", "   "},
	}}
	e := NewEditor(repo, nil)

	_, _, err := e.Commit(context.Background(), uuid.Nil, 1, Change{
		Actor: ActorCandidate, Origin: OriginEditor,
		Ops: []Op{{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"), Value: "Keep edited"}},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got := len(repo.state.Experience[0].Bullets); got != 1 {
		t.Fatalf("bullets = %d, want 1 after empty cleanup", got)
	}
}

// CommitDocument (the editor's autosave PATCH, and Reset from résumé) sanitizes the incoming
// state before diffing against what is stored — so the refuse check must run against the RAW
// next document, before that sanitize can truncate it away. A whole-document save that carries
// more than the cap for one role must be refused exactly like an agent's over-cap insert.
func TestCommitDocumentRefusesAWholeDocumentSaveOverTheCap(t *testing.T) {
	prev := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prev) })

	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	next := sample()
	bullets := make([]string, cv.MaxBullets+1)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Seeded bullet %d", i+1)
	}
	next.Experience[0].Bullets = bullets

	_, _, err := e.CommitDocument(context.Background(), repo.cvID, 1, ActorCandidate, OriginImport, next)
	if !errors.Is(err, ErrListCap) {
		t.Fatalf("CommitDocument = %v, want ErrListCap", err)
	}
	if repo.saves != 0 || len(repo.revisions) != 0 {
		t.Fatal("a refused whole-document save must not save or file a revision")
	}
	if got := len(repo.state.Experience[0].Bullets); got != 2 {
		t.Fatalf("stored bullets = %d, want the original 2 untouched", got)
	}
}

// SetRefuseListCap(false) is the ops kill switch (CV_EDIT_ALLOW_BULLET_TRUNCATION=true) and must
// restore the exact pre-refuse behaviour for a whole-document save too: Sanitize keeps the
// first MaxBullets and silently drops the rest.
func TestCommitDocumentAllowsOverCapWhenRefuseIsDisabled(t *testing.T) {
	prev := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prev) })

	repo := newFakeRepo()
	e := NewEditor(repo, nil).SetRefuseListCap(false)

	next := sample()
	bullets := make([]string, cv.MaxBullets+5)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Seeded bullet %d", i+1)
	}
	next.Experience[0].Bullets = bullets

	_, _, err := e.CommitDocument(context.Background(), repo.cvID, 1, ActorCandidate, OriginImport, next)
	if err != nil {
		t.Fatalf("CommitDocument with refuse off: %v", err)
	}
	if got := len(repo.state.Experience[0].Bullets); got != cv.MaxBullets {
		t.Fatalf("bullets = %d, want Sanitize to keep %d when refuse is off", got, cv.MaxBullets)
	}
}

// A whole-document save under the cap must not be affected by the new guard at all.
func TestCommitDocumentAllowsAWholeDocumentSaveUnderTheCap(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	next := sample()
	next.Summary = "Distributed systems"

	if _, _, err := e.CommitDocument(context.Background(), repo.cvID, 1, ActorCandidate, OriginEditor, next); err != nil {
		t.Fatalf("CommitDocument under the cap: %v", err)
	}
	if repo.state.Summary != "Distributed systems" {
		t.Fatalf("summary = %q, want the save applied", repo.state.Summary)
	}
}
