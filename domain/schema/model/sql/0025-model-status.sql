CREATE TABLE model_status_value (
    id INT PRIMARY KEY,
    status TEXT NOT NULL
);

-- Status values for the model.
INSERT INTO model_status_value VALUES
(0, 'error'),
(1, 'available'),
-- We set the model status to busy when the model is being upgraded.
(2, 'busy'),
(3, 'suspended');

CREATE TABLE model_status (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    status_id INT NOT NULL,
    message TEXT,
    updated_at DATETIME NOT NULL DEFAULT (STRFTIME('%Y-%m-%d %H:%M:%f', 'NOW', 'utc')),
    CONSTRAINT fk_model_status_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_status_status
    FOREIGN KEY (status_id)
    REFERENCES model_status_value (id)
);
