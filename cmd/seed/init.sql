CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS reference_vectors (
    id          BIGSERIAL PRIMARY KEY,
    vector      VECTOR(14) NOT NULL,
    is_fraud    BOOLEAN NOT NULL
);

-- The HNSW index is created by the seeding script, after the bulk COPY:
-- building it once on the loaded data is far faster than maintaining it
-- incrementally per-row.
--
-- We intentionally do NOT index is_fraud: the scoring query never filters on
-- it (it is only read from the nearest-neighbour rows), so a btree on a
-- low-cardinality boolean would add seed time and storage with no query win.
