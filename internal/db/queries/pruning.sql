-- Catalogue pruning: the queries that find, and eventually remove, jobs that do not
-- belong on an IT job board. See the catalog-pruning capability spec.

-- name: ResidualTitleGroups :many
-- The residual unclassified mass grouped into clusters an operator can act on: the
-- word groups occurring most often in the titles of live jobs that carry no is_tech
-- signal. Grouping is by word group, not by whole title, because boards append
-- location, schedule and requisition detail — half the unclassified mass has a title
-- that occurs exactly once, so whole-title grouping splits one role into singletons.
-- A word group is also the unit the non-tech dictionary accepts, so a reported
-- cluster is directly usable as an anchored term.
--
-- Two-word groups carry the ordinary case. Three-word groups exist only to bridge a
-- connector: in Portuguese and Spanish the preposition is part of the role name
-- ("operador de caixa"), while the two-word fragments around it ("analista de") are
-- noise, so a connector is allowed mid-group and never at an edge. Every other edge
-- token must be at least three characters and non-numeric, which is what keeps
-- shredded schedule notation ("m w", "2nd shift") out of the ranking.
--
-- Tokenizing on [^[:alnum:]]+ is Unicode-aware, so accented Latin and Cyrillic
-- survive whole — an ASCII class would split "Técnico" and lose the cluster.
-- Counting is DISTINCT over jobs, so a group repeated inside one verbose title does
-- not outrank a real cluster. Closed and duplicate rows are excluded: only a live,
-- canonical posting is worth a dictionary term.
WITH tok AS (
    SELECT j.id,
           j.source,
           regexp_split_to_array(lower(btrim(j.title)), '[^[:alnum:]]+') AS a
    FROM jobs j
    WHERE j.closed_at IS NULL
      AND j.duplicate_of IS NULL
      AND j.is_tech IS NULL
),
pos AS (
    SELECT tok.id, tok.source, tok.a, i
    FROM tok, generate_subscripts(tok.a, 1) AS i
),
grp AS (
    -- Two-word groups: both tokens must be role-bearing edges.
    SELECT DISTINCT id, source, (a[i] || ' ' || a[i + 1])::text AS grp
    FROM pos
    WHERE i + 1 <= cardinality(a)
      AND length(a[i])     >= 3 AND a[i]     !~ '^[0-9]+$'
      AND length(a[i + 1]) >= 3 AND a[i + 1] !~ '^[0-9]+$'
      AND a[i]     <> ALL(sqlc.arg(stop_words)::text[])
      AND a[i + 1] <> ALL(sqlc.arg(stop_words)::text[])
      AND a[i]     <> ALL(sqlc.arg(connectors)::text[])
      AND a[i + 1] <> ALL(sqlc.arg(connectors)::text[])
    UNION ALL
    -- Three-word groups: role-bearing edges bridging one connector.
    SELECT DISTINCT id, source, (a[i] || ' ' || a[i + 1] || ' ' || a[i + 2])::text
    FROM pos
    WHERE i + 2 <= cardinality(a)
      AND length(a[i])     >= 3 AND a[i]     !~ '^[0-9]+$'
      AND length(a[i + 2]) >= 3 AND a[i + 2] !~ '^[0-9]+$'
      AND a[i]     <> ALL(sqlc.arg(stop_words)::text[])
      AND a[i + 2] <> ALL(sqlc.arg(stop_words)::text[])
      AND a[i]     <> ALL(sqlc.arg(connectors)::text[])
      AND a[i + 2] <> ALL(sqlc.arg(connectors)::text[])
      AND a[i + 1]  = ANY(sqlc.arg(connectors)::text[])
)
SELECT grp,
       count(DISTINCT id)                 AS jobs,
       array_agg(DISTINCT source)::text[] AS sources
FROM grp
GROUP BY grp
ORDER BY 2 DESC, 1
LIMIT sqlc.arg(row_limit);
