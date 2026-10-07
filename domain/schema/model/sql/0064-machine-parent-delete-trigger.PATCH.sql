-- Notify a parent machine when its relationship to a child machine is
-- removed. The previous trigger ran after deleting the child machine row,
-- by which point the machine_parent row had already been explicitly
-- removed by the machine removal job, so the change-log insert never found
-- a row to forward and the parent machine was never notified that its
-- last child machine was gone.
DROP TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_delete;

-- noqa: disable=all
-- Namespace 3 is custom_machine_uuid_lifecycle_with_dependants and
-- edit type 4 is delete.
CREATE TRIGGER trg_log_custom_machine_uuid_lifecycle_with_dependants_machine_parent_delete
AFTER DELETE ON machine_parent FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES(4, 3, OLD.parent_uuid, DATETIME('now'));
END;
-- noqa: enable=all
