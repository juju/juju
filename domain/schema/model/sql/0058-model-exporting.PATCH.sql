-- This PATCH adds the model_exporting table.
-- An entry in this table indicates that the model is currently being
-- exported as part of a migration to another controller. It is the
-- counterpart to model_migrating, which tracks the importing side.

CREATE TABLE model_exporting (
    uuid TEXT NOT NULL PRIMARY KEY,
    model_uuid TEXT NOT NULL,
    CONSTRAINT fk_model_exporting_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid)
);

CREATE UNIQUE INDEX idx_model_exporting_model_uuid
ON model_exporting (model_uuid);
