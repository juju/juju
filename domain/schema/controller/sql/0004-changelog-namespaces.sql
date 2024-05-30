INSERT INTO change_log_namespace VALUES
    (0, 'external_controller', 'external controller changes based on the UUID'),
    (1, 'controller_node', 'controller node changes based on the controller ID'),
    (2, 'controller_config', 'controller config changes based on the key'),
    (3, 'model_migration_status', 'model migration status changes based on the UUID'),
    (4, 'model_migration_minion_sync', 'model migration minion sync changes based on the UUID'),
    (5, 'upgrade_info', 'upgrade info changes based on the UUID'),
    (6, 'cloud', 'cloud changes based on the UUID'),
    (7, 'cloud_credential', 'cloud credential changes based on the UUID'),
    (8, 'autocert_cache', 'autocert cache changes based on the UUID'),
    (9, 'upgrade_info_controller_node', 'upgrade info controller node changes based on the upgrade info UUID'),
    (10, 'object_store_metadata_path', 'object store metadata path changes based on the path'),
    (11, 'secret_backend_rotation', 'secret backend rotation changes based on the backend UUID and next rotation time'),
    (12, 'model', 'model changes based on the model UUID');
