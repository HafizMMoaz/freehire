-- Storage parameters for the ingest write path on jobs.
--
-- Measured on prod 2026-08-02, before the write path was fixed: 330,948,733 updates against
-- 5,608,085 live rows in eleven days — 59 per row — of which 37.7% could not go heap-only, so
-- each maintained all 21 indexes. The table carried 19.8% dead tuples and its TOAST relation
-- held ~63 GB against ~20 GB of live content: roughly 43 GB of a 102 GB database was bloat
-- rather than data.
--
-- fillfactor = 90 leaves room on a page for a HOT update, which is what the new cheap write
-- (RefreshUnchangedJob) needs to touch no index at all: it writes only last_seen_at, a column
-- deliberately in no index, but a heap-only update also requires free space on the row's own
-- page. jobs was filled by INSERTs at the default fillfactor = 100, so its existing pages have
-- none. This therefore takes effect GRADUALLY — only on pages written from here on — and fully
-- only after a repack, which is a separate change deliberately gated on this one's measured
-- effect. Expect the HOT share to climb over days, not at deploy.
--
-- autovacuum_vacuum_scale_factor = 0.02 replaces the default 0.2, which on a table this size
-- means autovacuum waits for ~1.1M dead tuples before it starts. The measured n_dead_tup was
-- 1.39M — the table sat permanently at the threshold, so cleanup ran late and in large bursts
-- against a host whose disk is already the constraint. 0.02 is the value companies already
-- carries (set when the same problem was found there), so this is the table matching its
-- neighbour rather than a new idea.
--
-- WHY a plain transactional migration, unlike 0071. This is a catalog update: no table scan, no
-- rewrite, ACCESS EXCLUSIVE held for microseconds. The hazard is only the WAIT — queueing
-- behind a long-running read and blocking every reader that arrives behind it — and that is
-- exactly what internal/migrate's lock_timeout bounds. 0071 needed no-transaction because
-- ADD CONSTRAINT ... CHECK HOLDS the lock across a full scan, which lock_timeout cannot help
-- with. Copying that shape here would be cargo cult.
--
-- Applied to a fresh volume by initdb after 0072; on an existing prod volume run this manually
-- (SET ROLE hire), on its own rather than batched with other migrations, so a lock_timeout abort
-- costs a retry of one statement and nothing else. It may run before or after the code deploy —
-- nothing reads these parameters, they only change how Postgres stores and vacuums the table.

ALTER TABLE public.jobs SET (
    fillfactor = 90,
    autovacuum_vacuum_scale_factor = 0.02
);
