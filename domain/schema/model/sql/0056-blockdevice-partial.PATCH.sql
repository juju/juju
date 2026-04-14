CREATE TABLE block_device_provenance (
    id INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO block_device_provenance VALUES
(0, 'provider'),
(1, 'machine');

-- TODO(merge): when merging this patch into main, add the provenance
-- column, but make it not null without a default.
ALTER TABLE block_device ADD COLUMN provenance INT NOT NULL DEFAULT 0
    REFERENCES block_device_provenance (id);
