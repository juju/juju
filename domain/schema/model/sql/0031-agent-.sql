
-- model_agent represents information about the agent that runs on behalf of a
-- model.
CREATE TABLE model_agent (
    model_uuid TEXT NOT NULL,
    password_hash_algorithm_id TEXT,
    password_hash TEXT,
    CONSTRAINT fk_model_uuid
    FOREIGN KEY (model_uuid)
    REFERENCES model (uuid),
    CONSTRAINT fk_model_agent_password_hash_algorithm
    FOREIGN KEY (password_hash_algorithm_id)
    REFERENCES password_hash_algorithm (id)
);
