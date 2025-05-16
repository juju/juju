// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)

const (
	// reservedCustomNamespaceIDOffset is the offset we use for standard
	// auto-generated namespaces to give ourselves space to add our own custom
	// namespaces. This is to prevent collisions with the trigger based
	// namespace IDs.
	// The namespace IDs indicate uniqueness not order, so we can safely have
	// gaps in the IDs.
	reservedCustomNamespaceIDOffset = 10000
)

type tableNamespaceID = int

// triggersForImmutableTable returns a function that creates triggers to prevent
// updates and deletes on the given table. The tableName is the name of the
// table to create the triggers for. The condition is an optional SQL condition
// that must be met for the trigger to be executed. The errMsg is the error
// message that will be returned if the trigger is fired.
func triggersForImmutableTable(tableName, condition, errMsg string) func() schema.Patch {
	if condition != "" {
		condition = fmt.Sprintf(`
    WHEN %s`[1:], condition)
	}
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
CREATE TRIGGER trg_%[1]s_immutable_update
    BEFORE UPDATE ON %[1]s
    FOR EACH ROW
%[2]s
    BEGIN
        SELECT RAISE(FAIL, '%[3]s');
    END;

CREATE TRIGGER trg_%[1]s_immutable_delete
    BEFORE DELETE ON %[1]s
    FOR EACH ROW
%[2]s
    BEGIN
        SELECT RAISE(FAIL, '%[3]s');
    END;`[1:], tableName, condition, errMsg)
		return schema.MakePatch(stmt)
	}
}

// triggersForUnmodifiableTable returns a function that creates triggers to
// prevent updates on the given table. The tableName is the name of the table to
// create the triggers for. The errMsg is the error message that
// will be returned if the trigger is fired.
func triggersForUnmodifiableTable(tableName, errMsg string) func() schema.Patch {
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
CREATE TRIGGER trg_%[1]s_immutable_update
    BEFORE UPDATE ON %[1]s
    FOR EACH ROW
    BEGIN
        SELECT RAISE(FAIL, '%[2]s');
    END;
`[1:], tableName, errMsg)
		return schema.MakePatch(stmt)
	}
}

// triggerGuardForTable returns a function that creates triggers to prevent
// updates on the given table when the specified condition (guard) is met. The
// tableName is the name of the table to create the triggers for. The condition
// is an optional SQL condition that must be met for the trigger to be executed.
// The errMsg is the error message that will be returned if the trigger is
// fired.
func triggerGuardForTable(tableName, condition, errMsg string) func() schema.Patch {
	return func() schema.Patch {
		stmt := fmt.Sprintf(`
CREATE TRIGGER trg_%[1]s_guard_update
    BEFORE UPDATE ON %[1]s
    FOR EACH ROW
        WHEN %[2]s
    BEGIN
        SELECT RAISE(FAIL, '%[3]s');
    END;`[1:], tableName, condition, errMsg)
		return schema.MakePatch(stmt)
	}
}
