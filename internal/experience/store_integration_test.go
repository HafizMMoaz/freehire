//go:build integration

// Integration test for the bank-to-profile skill sync (task 2.6 of
// unify-skills-via-experience-bank): banking an atom against a real Postgres actually
// updates the owner's search profile, and never creates one that did not exist. A fake
// ProfileSkills can prove the Store calls it; only a real userprofile.Service + Postgres
// can prove the whole path round-trips correctly. Run with:
// go test -tags=integration ./internal/experience/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package experience

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/testdb"
	"github.com/strelov1/freehire/internal/userprofile"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

func insertExperienceIntegrationUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func seedProfile(t *testing.T, pool *pgxpool.Pool, userID int64, skills []string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO user_profiles (user_id, specializations, skills) VALUES ($1, $2, $3)`,
		userID, []string{"backend"}, skills); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
}

func profileSkills(t *testing.T, pool *pgxpool.Pool, userID int64) ([]string, bool) {
	t.Helper()
	var skills []string
	err := pool.QueryRow(context.Background(),
		`SELECT skills FROM user_profiles WHERE user_id = $1`, userID).Scan(&skills)
	if err != nil {
		return nil, false
	}
	return skills, true
}

func TestStoreAddAtom_SyncsSkillsIntoAnExistingProfile(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertExperienceIntegrationUser(t, pool, "bank-sync-existing@example.test")
	seedProfile(t, pool, userID, []string{"go"})

	profileSvc := userprofile.New(userprofile.NewQueriesRepository(queries))
	bank := NewStore(NewQueriesRepository(queries)).SetProfileSkills(profileSvc)

	if _, err := bank.AddAtom(ctx, userID, Atom{
		Claim:      "Migrated the cluster to Kubernetes",
		Skills:     []string{"k8s"},
		Provenance: ProvenanceManual,
	}); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	skills, ok := profileSkills(t, pool, userID)
	if !ok {
		t.Fatal("profile row disappeared")
	}
	found := false
	for _, s := range skills {
		if s == "kubernetes" {
			found = true
		}
	}
	if !found {
		t.Errorf("profile skills = %v, want to include the canonicalized kubernetes", skills)
	}
	if len(skills) != 2 {
		t.Errorf("profile skills = %v, want exactly go+kubernetes (existing skill untouched, no duplication)", skills)
	}
}

func TestStoreAddAtom_LeavesNoProfileBehindForAUserWithNone(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertExperienceIntegrationUser(t, pool, "bank-sync-noprofile@example.test")

	profileSvc := userprofile.New(userprofile.NewQueriesRepository(queries))
	bank := NewStore(NewQueriesRepository(queries)).SetProfileSkills(profileSvc)

	if _, err := bank.AddAtom(ctx, userID, Atom{
		Claim:      "Migrated the cluster to Kubernetes",
		Skills:     []string{"k8s"},
		Provenance: ProvenanceManual,
	}); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	if _, ok := profileSkills(t, pool, userID); ok {
		t.Error("a profile row was created as a side effect of banking an atom — it must not be")
	}
}
