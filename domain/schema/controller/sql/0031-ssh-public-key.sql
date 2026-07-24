-- Add a column to store the marshalled public host key alongside the private
-- key. The public key is derived once at bootstrap and stored, so it can be
-- served to clients without re-parsing the private key on every retrieval.
ALTER TABLE controller_ssh_host_key
ADD COLUMN public_key BLOB NOT NULL DEFAULT x'';
