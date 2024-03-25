// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	corepermission "github.com/juju/juju/core/permission"
	permissionerrors "github.com/juju/juju/domain/permission/errors"
	internaldatabase "github.com/juju/juju/internal/database"
)

// SharedState holds the shared state for the permission domain. Methods
// which are shared between multiple domains.
type SharedState struct {
	statementBase internaldatabase.StatementBase
}

// NewSharedState creates a new SharedState.
func NewSharedState(base internaldatabase.StatementBase) *SharedState {
	return &SharedState{statementBase: base}
}

// AddUserPermissionArgs is a specification for adding a user permission.
type AddUserPermissionArgs struct {
	PermissionUUID string
	UserUUID       string
	Access         corepermission.Access
	Target         corepermission.ID
}

// AddUserPermission adds a permission for the given user on the given target.
func (s SharedState) AddUserPermission(ctx context.Context, tx *sqlair.TX, spec AddUserPermissionArgs) error {
	// Insert a permission doc with
	// * permissionObjectAccess as permission_type_id
	// * uuid of the user (spec.User) as grant_to
	// * spec.Target.Key as grant_on
	newPermission := `
INSERT INTO permission (uuid, permission_type_id, grant_on, grant_to)
VALUES ($addUserPermission.uuid, $addUserPermission.permission_type_id, $addUserPermission.grant_on, $addUserPermission.grant_to)
`
	insertPermissionStmt, err := s.statementBase.Prepare(newPermission, addUserPermission{})
	if err != nil {
		return errors.Trace(err)
	}

	accessTypeID, err := objectAccessID(ctx, tx, spec.Access, spec.Target.ObjectType)
	if err != nil {
		return errors.Trace(err)
	}

	perm := addUserPermission{
		UUID:           spec.PermissionUUID,
		GrantOn:        spec.Target.Key,
		GrantTo:        spec.UserUUID,
		PermissionType: accessTypeID,
	}

	if err = validateTargetExists(ctx, tx, spec.Target.Key); err != nil {
		return errors.Trace(err)
	}

	// No IsErrConstraintForeignKey should be seen as both foreign keys
	// have been checked.
	err = tx.Query(ctx, insertPermissionStmt, perm).Run()
	if internaldatabase.IsErrConstraintUnique(err) {
		return errors.Annotatef(permissionerrors.AlreadyExists, "%q on %q", spec.UserUUID, spec.Target.Key)
	} else if err != nil {
		return errors.Annotatef(err, "adding permission %q for %q on %q", spec.Access, spec.UserUUID, spec.Target.Key)
	}

	return nil
}
