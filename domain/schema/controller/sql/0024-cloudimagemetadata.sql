CREATE TABLE cloud_image_metadata (
    uuid TEXT NOT NULL PRIMARY KEY,
    created_at DATETIME NOT NULL,
    source TEXT NOT NULL,
    stream TEXT NOT NULL,
    region TEXT NOT NULL,
    version TEXT NOT NULL,
    architecture_id INT NOT NULL,
    virt_type TEXT NOT NULL,
    root_storage_type TEXT NOT NULL,
    root_storage_size INT,
    priority INT,
    image_id TEXT NOT NULL,
    CONSTRAINT fk_cloud_image_metadata_arch
    FOREIGN KEY (architecture_id)
    REFERENCES architecture (id)
);

CREATE UNIQUE INDEX idx_cloud_image_metadata_unique_fields
ON cloud_image_metadata (stream, region, version, architecture_id, virt_type, root_storage_type, source);
