CREATE TABLE sequence (
    namespace TEXT NOT NULL PRIMARY KEY,
    value INT NOT NULL
);

CREATE INDEX idx_sequence_namespace_value
ON sequence (namespace, value);
