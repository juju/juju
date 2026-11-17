-- Patch 0067: identify which agent login recorded the presence.
--
-- An agent can log in again, on any controller, before the API server sees
-- its old connection close. The old connection must then not remove the
-- presence recorded by the new login. Each login writes a new token, and a
-- connection only removes the presence if it still holds its token.
ALTER TABLE unit_agent_presence ADD COLUMN token TEXT;
ALTER TABLE machine_agent_presence ADD COLUMN token TEXT;
