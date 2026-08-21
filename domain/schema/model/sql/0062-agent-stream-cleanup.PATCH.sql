-- Remove agent-stream and agent-version keys from model_config.
-- These values are stored in the agent_version table and are
-- synthesized by the v_model_config view. Storing them in model_config
-- causes the view to return conflicting rows.
DELETE FROM model_config
WHERE "key" IN ('agent-stream', 'agent-version');
