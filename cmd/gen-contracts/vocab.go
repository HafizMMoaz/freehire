package main

import (
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/collections"
)

// emitVocab renders one closed vocabulary as a frozen value array plus a string-union
// type, e.g. emitVocab("Seniority", "SENIORITY_VALUES", […]) →
//
//	export const SENIORITY_VALUES = ['intern', …] as const;
//	export type Seniority = (typeof SENIORITY_VALUES)[number];
func emitVocab(typeName, constName string, values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + v + "'"
	}
	var b strings.Builder
	b.WriteString("export const " + constName + " = [" + strings.Join(quoted, ", ") + "] as const;\n")
	b.WriteString("export type " + typeName + " = (typeof " + constName + ")[number];\n")
	return b.String()
}

// emitMap renders a string→string map as a frozen object literal plus a type alias,
// e.g. emitMap("CityCountry", "CITY_COUNTRY_MAP", {…}) →
//
//	export const CITY_COUNTRY_MAP = {
//	  'Amsterdam': 'nl',
//	  …
//	} as const;
//	export type CityCountry = typeof CITY_COUNTRY_MAP;
//
// Keys are emitted in sorted order so the committed output is deterministic despite
// Go's randomized map iteration.
func emitMap(typeName, constName string, m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if len(keys) == 0 {
		b.WriteString("export const " + constName + " = {} as const;\n")
	} else {
		b.WriteString("export const " + constName + " = {\n")
		for _, k := range keys {
			b.WriteString("  " + quoteTS(k) + ": " + quoteTS(m[k]) + ",\n")
		}
		b.WriteString("} as const;\n")
	}
	b.WriteString("export type " + typeName + " = typeof " + constName + ";\n")
	return b.String()
}

// emitMapOfSlices renders a string→[]string map as a frozen object of string
// arrays plus a type alias. Keys and each value slice are emitted in sorted order
// so the committed output is deterministic despite Go's randomized map iteration.
func emitMapOfSlices(typeName, constName string, m map[string][]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if len(keys) == 0 {
		b.WriteString("export const " + constName + " = {} as const;\n")
	} else {
		b.WriteString("export const " + constName + " = {\n")
		for _, k := range keys {
			vals := append([]string(nil), m[k]...)
			sort.Strings(vals)
			quoted := make([]string, len(vals))
			for i, v := range vals {
				quoted[i] = quoteTS(v)
			}
			b.WriteString("  " + quoteTS(k) + ": [" + strings.Join(quoted, ", ") + "],\n")
		}
		b.WriteString("} as const;\n")
	}
	b.WriteString("export type " + typeName + " = typeof " + constName + ";\n")
	return b.String()
}

// quoteTS renders a string as a single-quoted TS literal, escaping backslashes and
// single quotes so a value like "N'Djamena" can't break the generated file.
func quoteTS(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// emitCollections renders the company-tag registry as a frozen array of objects
// plus the two types the frontend reads off it:
//
//	export const COLLECTIONS = [
//	  { slug: 'yc', title: 'Y Combinator', description: '…', kind: 'editorial' },
//	  …
//	] as const;
//	export type Collection = (typeof COLLECTIONS)[number];
//	export type CollectionKind = Collection['kind'];
//
// Emitted in registry order, not sorted: that order is the display order on the
// /collections hub and in the job-search facet.
//
// This replaces a hand-kept mirror in web/src/lib/collections.ts. `kind` decides
// which filter group a tag renders in, so a slug missing from the frontend copy
// meant a tag that filters but has no pill — a correctness bug rather than stale
// copy, which is what makes it worth generating.
func emitCollections(all []collections.Collection) string {
	var b strings.Builder
	b.WriteString("export const COLLECTIONS = [\n")
	for _, c := range all {
		b.WriteString("  { slug: " + quoteTS(c.Slug) +
			", title: " + quoteTS(c.Title) +
			", description: " + quoteTS(c.Description) +
			", kind: " + quoteTS(string(c.Kind)) + " },\n")
	}
	b.WriteString("] as const;\n")
	b.WriteString("export type Collection = (typeof COLLECTIONS)[number];\n")
	b.WriteString("export type CollectionKind = Collection['kind'];\n")
	return b.String()
}
