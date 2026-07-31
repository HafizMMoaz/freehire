package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/collections"
)

func TestGenVocabEmitsRoleLabels(t *testing.T) {
	got := genVocab()
	if !strings.Contains(got, "export const ROLE_LABELS = {") {
		t.Errorf("genVocab() missing ROLE_LABELS map:\n%s", got)
	}
	// The catalog is the source of truth for picker labels — a named role must
	// carry its human label.
	if !strings.Contains(got, "'founding_engineer': 'Founding Engineer'") {
		t.Errorf("genVocab() ROLE_LABELS missing founding_engineer label")
	}
}

func TestEmitVocab(t *testing.T) {
	got := emitVocab("Seniority", "SENIORITY_VALUES", []string{"junior", "senior"})
	want := "export const SENIORITY_VALUES = ['junior', 'senior'] as const;\n" +
		"export type Seniority = (typeof SENIORITY_VALUES)[number];\n"
	if got != want {
		t.Errorf("emitVocab mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestEmitVocabEmpty(t *testing.T) {
	got := emitVocab("X", "X_VALUES", nil)
	want := "export const X_VALUES = [] as const;\n" +
		"export type X = (typeof X_VALUES)[number];\n"
	if got != want {
		t.Errorf("emitVocab(empty) = %q, want %q", got, want)
	}
}

func TestEmitMap(t *testing.T) {
	// Keys must be emitted in sorted order — the output is committed, so it has to be
	// deterministic regardless of Go's random map iteration.
	got := emitMap("CityCountry", "CITY_COUNTRY_MAP", map[string]string{"Berlin": "de", "Amsterdam": "nl"})
	want := "export const CITY_COUNTRY_MAP = {\n" +
		"  'Amsterdam': 'nl',\n" +
		"  'Berlin': 'de',\n" +
		"} as const;\n" +
		"export type CityCountry = typeof CITY_COUNTRY_MAP;\n"
	if got != want {
		t.Errorf("emitMap mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestEmitMapEmpty(t *testing.T) {
	got := emitMap("X", "X_MAP", nil)
	want := "export const X_MAP = {} as const;\n" +
		"export type X = typeof X_MAP;\n"
	if got != want {
		t.Errorf("emitMap(empty) = %q, want %q", got, want)
	}
}

func TestEmitCollections_RendersTheRegistryWithItsKinds(t *testing.T) {
	got := emitCollections([]collections.Collection{
		{Slug: "yc", Title: "Y Combinator", Description: "Open roles at YC companies.", Kind: collections.KindEditorial},
		{Slug: "uk-skilled-worker-sponsor", Title: "Licensed UK sponsor", Description: "It's a licence.", Kind: collections.KindCredential},
	})

	for _, want := range []string{
		"export const COLLECTIONS = [",
		"{ slug: 'yc', title: 'Y Combinator', description: 'Open roles at YC companies.', kind: 'editorial' },",
		"{ slug: 'uk-skilled-worker-sponsor', title: 'Licensed UK sponsor', description: 'It\\'s a licence.', kind: 'credential' },",
		"] as const;",
		"export type Collection = (typeof COLLECTIONS)[number];",
		"export type CollectionKind = Collection['kind'];",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("emitCollections output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestEmitCollections_KeepsRegistryOrder(t *testing.T) {
	// Display order is the registry's, not alphabetical — the hub and the facet
	// render in it.
	got := emitCollections([]collections.Collection{
		{Slug: "zeta", Kind: collections.KindEditorial},
		{Slug: "alpha", Kind: collections.KindEditorial},
	})
	if strings.Index(got, "'zeta'") > strings.Index(got, "'alpha'") {
		t.Error("emitCollections reordered the registry")
	}
}
