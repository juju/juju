CREATE TABLE model_agent (
    model_uuid TEXT NOT NULL PRIMARY KEY,

    -- previous_version describes the agent version that was in use before the
    -- the current target_version.
    previous_version TEXT NOT NULL,

    -- target_version describes the desired agent version that should be
    -- being run in this model. It should not be considered "the" version that
    -- is being run for every agent as each agent needs to upgrade to this
    -- version.
    target_version TEXT NOT NULL,
    CONSTRAINT fk_model_agent_model
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid)
);
