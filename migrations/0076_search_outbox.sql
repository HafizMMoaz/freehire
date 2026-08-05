-- Incremental facet-search queue. Mirrors enrichment_outbox: a reference-only outbox
-- (job_id + lease/retry bookkeeping), drained by the cmd/search-drain worker, which
-- pushes a whole claimed wave into the live `jobs` Meili index as ONE batch.
--
-- Replaces the previous design where cmd/ingest pushed each crawl's changed jobs
-- straight into the live index (search.Client.SubmitJobs) from inside every one of the
-- ~169 per-board worker processes. Each push, however small, forced Meilisearch to
-- re-merge its inverted index/facet structures across the whole multi-million-document
-- live index — observed live at 50-90s per push, essentially continuously, saturating
-- host disk IO and starving freehire-web's accept() calls (nginx 504 "while connecting
-- to upstream"). Routing every write through one outbox lets a single worker collapse
-- however many boards changed in a window into one Meili task, instead of one task per
-- board per crawl.
--
-- No target-version/target-model column (unlike enrichment_outbox/semantic_outbox):
-- the facet index has no staleness key beyond content_hash, which cmd/ingest's cheap-
-- write gate already checks before enqueuing, so one live entry per job is enough.
--
-- Applied to a fresh volume by initdb after 0075; on an existing prod volume this
-- statement must be run manually BEFORE deploying code that writes to the table.

CREATE TABLE public.search_outbox (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    claimed_at timestamp with time zone,
    failed_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.search_outbox ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.search_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.search_outbox
    ADD CONSTRAINT search_outbox_pkey PRIMARY KEY (id);

-- One live entry per job: the enqueue dedup key, so a job re-changed before it drains
-- is not queued twice.
ALTER TABLE ONLY public.search_outbox
    ADD CONSTRAINT search_outbox_job_id_key UNIQUE (job_id);

ALTER TABLE ONLY public.search_outbox
    ADD CONSTRAINT search_outbox_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;

-- Partial index over claimable (not dead-lettered) entries, mirroring
-- enrichment_outbox_claimable_idx.
CREATE INDEX search_outbox_claimable_idx ON public.search_outbox USING btree (id) WHERE (failed_at IS NULL);
