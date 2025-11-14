-- When merging into main, update the file domain/schema/controller.go:
--  - triggersForImmutableTable "secret_backend" needs to be updated to below condition and message
--  - this file needs to be removed

DROP TRIGGER IF EXISTS trg_secret_backend_immutable_update;

CREATE TRIGGER trg_secret_backend_immutable_update
    BEFORE UPDATE ON secret_backend
    FOR EACH ROW
    WHEN OLD.name IN ('internal', 'kubernetes')
BEGIN
    SELECT RAISE(FAIL, 'built-in secret backends named "internal" or "kubernetes" are immutable');
END;


DROP TRIGGER IF EXISTS trg_secret_backend_immutable_delete;

CREATE TRIGGER trg_secret_backend_immutable_delete
    BEFORE DELETE ON secret_backend
    FOR EACH ROW
    WHEN OLD.name IN ('internal', 'kubernetes')
BEGIN
    SELECT RAISE(FAIL, 'built-in secret backends named "internal" or "kubernetes" are immutable');
END;

