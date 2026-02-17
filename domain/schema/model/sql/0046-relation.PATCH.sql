/**
  IMPORTANT FOR MERGE:

  This trigger can be removed during the merge, so that it is just a constraint
  check on the relation.uuid column.
 */

-- noqa: disable=all
CREATE TRIGGER trg_custom_relation_uuid_empty_constraint
BEFORE INSERT ON relation FOR EACH ROW
WHEN
    (NEW.uuid = '')
BEGIN
    SELECT RAISE(FAIL, 'relation.uuid cannot be NULL or empty');
END;
