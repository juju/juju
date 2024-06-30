-- Note: once released, this file should not be modified.
--      If a new change log namespace is needed, a new file should be created.
INSERT INTO change_log_namespace VALUES
(0, 'model_config', 'Model configuration changes based on key'),
(1, 'object_store_metadata_path', 'Object store metadata path changes based on the path'),
(2, 'block_device', 'Block device changes based on the machine UUID'),
(3, 'storage_attachment', 'Storage attachment changes based on the storage instance UUID'),
(4, 'storage_filesystem', 'File system changes based on UUID'),
(5, 'storage_filesystem_attachment', 'File system attachment changes based on UUID'),
(6, 'storage_volume', 'Storage volume changes based on UUID'),
(7, 'storage_volume_attachment', 'Volume attachment changes based on UUID'),
(8, 'storage_volume_attachment_plan', 'Volume attachment plan changes based on UUID'),
(9, 'secret_metadata', 'Secret auto prune changes based on UUID'),
(10, 'secret_rotation', 'Secret rotation changes based on UUID'),
(11, 'secret_revision_obsolete', 'Secret revision obsolete changes based on UUID'),
(12, 'secret_revision_expire', 'Secret revision next expire time changes based on UUID'),
(13, 'secret_revision', 'Secret revision changes based on UUID'),
(14, 'secret_reference', 'Secret reference changes based on UUID'),
(15, 'subnet', 'Subnet changes based on UUID'),
(16, 'machine', 'Machine changes based on UUID'),
(17, 'user_public_ssh_key', 'User public ssh key changes based on id');
