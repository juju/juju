-- The model_config table is a new table that is used to store configuration 
-- data for the model.
--
-- The provider tracker relies on the model_config table. Do not modify the
-- model_config table in a patch/build release. Only make changes to this table
-- during a major/minor release.
CREATE TABLE model_config (
    "key" TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

-- The model_constraint table is a new table that is used to store the
-- constraints that are associated with a model.
CREATE TABLE model_constraint (
    model_uuid TEXT NOT NULL PRIMARY KEY,
    constraint_uuid TEXT NOT NULL,
    CONSTRAINT fk_model_constraint_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_constraint_constraint
    FOREIGN KEY (constraint_uuid)
    REFERENCES "constraint" (uuid)
);

-- v_model_constraint is a view to represent the current model constraints. If
-- no constraints have been set then expect this view to be empty. There will
-- also only ever be a maximum of 1 record in this view.
CREATE VIEW v_model_constraint AS
SELECT c.*
FROM model_constraint mc
INNER JOIN v_constraint c ON mc.constraint_uuid = c.uuid;