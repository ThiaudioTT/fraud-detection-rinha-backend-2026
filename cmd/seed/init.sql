CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS reference_vectors (
    id          BIGSERIAL PRIMARY KEY,
    vector      VECTOR(14) NOT NULL,
    is_fraud    BOOLEAN NOT NULL
);

-- We creating the index on the seeding script! 
-- Building the HNSW index is much faster after bulk loading the data, rather than updating it incrementally per-row.
-- CREATE INDEX IF NOT EXISTS reference_vectors_l2_idx
--     ON reference_vectors USING hnsw (vector vector_l2_ops)
--     WITH (m = 8, ef_construction = 32);

CREATE INDEX IF NOT EXISTS reference_vectors_is_fraud_idx
    ON reference_vectors (is_fraud);
