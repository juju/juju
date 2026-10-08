-- Backup requests share a controller-wide lease across API server nodes.
INSERT INTO lease_type VALUES (2, 'backup-creation');
