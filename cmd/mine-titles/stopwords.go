package main

// The miner ranks word groups drawn from job titles, and two classes of word have
// to be handled differently or the ranking fills with noise.
//
// stopWords never carry a role — employment type, schedule notation and posting
// boilerplate. A measurement against prod put "m w"/"w d"/"h f" (the shrapnel of
// M-W-F schedules) and "part time" above every real role, and they alone accounted
// for most of the apparent coverage. No word group may contain one, anywhere.
//
// The doctrine matches internal/classify: curated, never inferred, deliberately
// conservative. A word here silently removes every group containing it, so a term
// that is part of a real role phrase would hide that whole family from mining
// rather than merely filter it — "home" would cost "home health aide", "call" would
// cost "call center". A test asserts no entry is a token of any classify non-tech
// term; keep it that way when adding.
var stopWords = []string{
	// Pronouns — never part of a role
	"you", "our", "your", "we",

	// Employment type, schedule and work arrangement — a job attribute, not a role
	"full", "part", "time", "shift", "hour", "hours", "day", "days", "week",
	"weekly", "weekday", "weekdays", "weekend", "weekends", "night", "nights",
	"evening", "morning", "prn", "temp", "temporary", "permanent", "contract",
	"casual", "seasonal", "remote", "onsite", "hybrid", "travel",

	// Posting boilerplate — the words a board wraps around the role
	"new", "needed", "hiring", "urgent", "immediate", "immediately", "now", "join",
	"job", "jobs", "position", "positions", "opening", "openings", "opportunity",
	"career", "careers", "apply", "req", "experience", "required", "sign", "bonus",
	"pay", "salary", "rate", "level", "based", "genders",
}

// connectors are function words that belong INSIDE a role phrase but never at its
// edge. Romance-language titles make this distinction load-bearing: in Portuguese
// and Spanish the preposition is part of the role name — "operador de caixa",
// "auxiliar de limpeza", "técnico de enfermagem" are all in the classify non-tech
// dictionary — while a pair ending or beginning in the same word ("analista de",
// "banco de") is a fragment. Treating them as ordinary stop words would make the
// whole Portuguese and Spanish tail of the catalogue unmineable; treating them as
// ordinary words would fill the ranking with fragments.
//
// So a group is kept only when no connector sits at either end. A two-word group
// therefore cannot contain one at all, which is why the miner also builds
// three-word groups: they are the shortest unit that can carry a connector.
var connectors = []string{
	// English
	"of", "in", "at", "to", "for", "and", "or", "the", "with", "per", "from", "a", "an",
	// Portuguese and Spanish
	"de", "da", "do", "dos", "das", "em", "para", "com", "del", "la", "el", "los",
	"las", "por", "que", "y",
	// German
	"der", "die", "und", "fur",
	// Russian
	"по", "на", "для", "или", "под", "и", "с",
	// Danish and Norwegian — a prod run surfaced "søges til" ("wanted for") in the
	// top 100, which is a posting phrase, not a role. Danish "for" is spelled like
	// the English one and is already listed above.
	"til", "med", "hos",
}
