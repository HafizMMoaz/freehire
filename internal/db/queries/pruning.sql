-- Catalogue pruning: the queries that find, and eventually remove, jobs that do not
-- belong on an IT job board. See the catalog-pruning capability spec.

-- name: ResidualUnclassifiedTitles :many
-- The residual unclassified mass grouped into clusters an operator can act on: the
-- most frequent titles among live jobs carrying no is_tech signal. Titles are
-- normalized (lowercased, trimmed) so one role spelled inconsistently across boards
-- reads as one cluster rather than several singletons. Closed and duplicate rows are
-- excluded — only a live, canonical posting is worth a dictionary term.
SELECT
    lower(btrim(title))                AS title,
    count(*)                           AS jobs,
    array_agg(DISTINCT source)::text[] AS sources
FROM jobs
WHERE closed_at IS NULL
  AND duplicate_of IS NULL
  AND is_tech IS NULL
GROUP BY 1
ORDER BY 2 DESC, 1
LIMIT sqlc.arg(row_limit);
