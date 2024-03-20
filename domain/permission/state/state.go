// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	coredatabase "github.com/juju/juju/core/database"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/permission"
	permissionerrors "github.com/juju/juju/domain/permission/errors"
	usererrors "github.com/juju/juju/domain/user/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/uuid"
)

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new state reference.
func NewState(factory coredatabase.TxnRunnerFactory, logger Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// CreatePermission gives the user access per the provided spec.
// It requires the user/target combination has not already been
// created. UserAccess is returned on success.
// If the user provided does not exist or is marked removed,
// usererrors.NotFound is returned.
// If the user provided exists but is marked disabled,
// usererrors.AuthenticationDisabled is returned.
// If a permission for the user and target key already exists,
// permissionerrors.AlreadyExists is returned.
func (s *State) CreatePermission(ctx context.Context, newPermissionUUID uuid.UUID, spec permission.UserAccessSpec) (corepermission.UserAccess, error) {
	var userAccess corepermission.UserAccess

	db, err := s.DB()
	if err != nil {
		return userAccess, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		user, err := findUser(ctx, tx, spec.User)
		if err != nil {
			return errors.Trace(err)
		}

		if err := AddUserPermission(ctx, tx, AddUserPermissionArgs{
			PermissionUUID: newPermissionUUID.String(),
			UserUUID:       user.UUID,
			User:           spec.User,
			Access:         spec.Access,
			Target:         spec.Target,
		}); err != nil {
			return errors.Trace(err)
		}

		userAccess = user.toCoreUserAccess()

		return nil
	})
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(domain.CoerceError(err))
	}

	userAccess.Access = spec.Access
	userAccess.PermissionID = newPermissionUUID.String()
	userAccess.Object = objectTag(spec.Target)
	return userAccess, nil
}

// AddUserPermissionArgs is a specification for adding a user permission.
type AddUserPermissionArgs struct {
	PermissionUUID string
	UserUUID       string
	User           string
	Access         corepermission.Access
	Target         corepermission.ID
}

// AddUserPermission adds a permission for the given user on the given target.
func AddUserPermission(ctx context.Context, tx *sqlair.TX, spec AddUserPermissionArgs) error {
	// Insert a permission doc with
	// * permissionObjectAccess as permission_type_id
	// * uuid of the user (spec.User) as grant_to
	// * spec.Target.Key as grant_on
	newPermission := `
INSERT INTO permission (uuid, permission_type_id, grant_on, grant_to)
VALUES ($Permission.uuid, $Permission.permission_type_id, $Permission.grant_on, $Permission.grant_to)
`
	insertPermissionStmt, err := sqlair.Prepare(newPermission, Permission{})
	if err != nil {
		return errors.Trace(err)
	}

	perm := Permission{
		UUID:    spec.PermissionUUID,
		GrantOn: spec.Target.Key,
		GrantTo: spec.UserUUID,
	}

	accessTypeID, err := objectAccessID(ctx, tx, spec.Access, spec.Target.ObjectType)
	if err != nil {
		return errors.Trace(err)
	}
	perm.PermissionType = accessTypeID

	if err = validateTargetExists(ctx, tx, spec.Target.Key); err != nil {
		return errors.Trace(err)
	}

	// No IsErrConstraintForeignKey should be seen as both foreign keys
	// have been checked.
	err = tx.Query(ctx, insertPermissionStmt, perm).Run()
	if internaldatabase.IsErrConstraintUnique(err) {
		return errors.Annotatef(permissionerrors.AlreadyExists, "%q on %q", spec.User, spec.Target.Key)
	} else if err != nil {
		return errors.Annotatef(err, "adding permission %q for %q on %q", spec.Access, spec.User, spec.Target.Key)
	}

	return nil
}

// DeletePermission removes the given subject's (user) access to the
// given target.
// If the specified subject does not exist, a usererrors.NotFound is
// returned.
// If the permission does not exist, no error is returned.
func (s *State) DeletePermission(ctx context.Context, subject string, target corepermission.ID) error {
	// TODO: is target.Key sufficient to Delete a permission?
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	m := make(sqlair.M, 1)
	m["grant_on"] = target.Key

	// The combination of grant_to and grant_on are guaranteed to be
	// unique, thus it is all that is deleted to select the row to be
	// deleted.
	deletePermission := `
DELETE 
FROM permission 
WHERE grant_to = $M.grant_to AND grant_on = $M.grant_on
`
	deletePermissionStmt, err := sqlair.Prepare(deletePermission, m)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		userUUID, err := userUUIDForName(ctx, tx, subject)
		if err != nil {
			return errors.Annotatef(usererrors.NotFound, "looking up UUID for user %q", subject)
		}
		m["grant_to"] = userUUID

		err = tx.Query(ctx, deletePermissionStmt, m).Run()
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "deleting permission of %q on %q", subject, target.Key)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// UpsertPermission updates the permission on the target for the given
// subject (user). The api user must have Admin permission on the target. If a
// subject does not exist, it is created using the subject and api user. Access
// can be granted or revoked.
func (s *State) UpsertPermission(ctx context.Context, args permission.UpsertPermissionArgs) error {
	return errors.NotImplementedf("UpsertPermission")
}

// ReadUserAccessForTarget returns the subject's (user) access for the
// given user on the given target.
func (s *State) ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error) {
	return corepermission.UserAccess{}, errors.NotImplementedf("ReadUserAccessForTarget")
}

// ReadUserAccessLevelForTarget returns the subject's (user) access level
// for the given user on the given target.
func (s *State) ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error) {
	return corepermission.NoAccess, errors.NotImplementedf("ReadUserAccessLevelForTarget")
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// subject's (user) has for any access type.
func (s *State) ReadAllUserAccessForUser(ctx context.Context, subject string) ([]corepermission.UserAccess, error) {
	return nil, errors.NotImplementedf("ReadAllUserAccessForUser")
}

// ReadAllUserAccessForTarget return a slice of user access for all users
// with access to the given target.
func (s *State) ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error) {
	return nil, errors.NotImplementedf("ReadAllUserAccessForTarget")
}

// ReadAllAccessTypeForUser return a slice of user access for the subject
// (user) specified and of the given access type.
// E.G. All clouds the user has access to.
func (s *State) ReadAllAccessTypeForUser(ctx context.Context, subject string, access_type corepermission.ObjectType) ([]corepermission.UserAccess, error) {
	return nil, errors.NotImplementedf("ReadAllAccessTypeForUser")
}

// findUser finds the user provided exists, hasn't been removed and is not
// disabled. Return data needed to fill in corePermission.UserAccess.
func findUser(
	ctx context.Context,
	tx *sqlair.TX,
	userName string,
) (User, error) {
	var result User

	getUserQuery := `
SELECT (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&User.*),
       creator.name AS &User.created_by_name
FROM   v_user_auth u
    LEFT JOIN user AS creator
    ON        u.created_by_uuid = creator.uuid
WHERE  u.removed = false AND u.name = $M.name`

	selectUserStmt, err := sqlair.Prepare(getUserQuery, User{}, sqlair.M{})
	if err != nil {
		return result, errors.Annotate(err, "preparing select getUser query")
	}

	err = tx.Query(ctx, selectUserStmt, sqlair.M{"name": userName}).Get(&result)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return result, errors.Annotatef(usererrors.NotFound, "%q", userName)
	} else if err != nil {
		return result, errors.Annotatef(err, "getting user with name %q", userName)
	}
	if result.Disabled {
		return result, errors.Annotatef(usererrors.AuthenticationDisabled, "%q", userName)
	}
	return result, nil
}

// userUUIDForName returns the user UUID for the associated name
// if the user is active.
// Method borrowed from the user domain state.
func userUUIDForName(
	ctx context.Context, tx *sqlair.TX, name string,
) (string, error) {
	stmt, err := sqlair.Prepare(
		`SELECT &M.uuid FROM user WHERE name = $M.name`, sqlair.M{})

	if err != nil {
		return "", errors.Annotate(err, "preparing user UUID statement")
	}

	var inOut = sqlair.M{"name": name}
	err = tx.Query(ctx, stmt, inOut).Get(&inOut)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.Annotatef(usererrors.NotFound, "active user %q", name)
		}
		return "", errors.Annotatef(err, "getting user %q", name)
	}

	uuid, _ := inOut["uuid"].(string)
	return uuid, nil
}

func objectAccessID(
	ctx context.Context,
	tx *sqlair.TX,
	access corepermission.Access,
	objectType corepermission.ObjectType,
) (int64, error) {
	// id of spec.Access from permission_access_type as access_type_id
	// id of spec.Target.ObjectType from permission_object_type as object_type_id
	// Use access_type_id and object_type_id to validate row from permission_object_access
	objectAccessIDExists := `
SELECT permission_access_type.id AS &M.access_type_id
FROM permission_object_access
LEFT JOIN permission_object_type
	ON permission_object_type.type = $M.object_type
LEFT JOIN permission_access_type
	ON permission_access_type.type = $M.access_type
WHERE permission_object_access.access_type_id = permission_access_type.id
	AND permission_object_access.object_type_id = permission_object_type.id
`

	// Validate the access type is allowed for the target type.
	objectAccessIDStmt, err := sqlair.Prepare(objectAccessIDExists, sqlair.M{})
	if err != nil {
		return -1, errors.Trace(err)
	}

	var resultM = sqlair.M{}
	err = tx.Query(ctx, objectAccessIDStmt, sqlair.M{"access_type": access, "object_type": objectType}).Get(&resultM)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return -1, errors.Annotatef(err, "mismatch in %q, %q", access, objectType)
	} else if err != nil {
		return -1, errors.Annotatef(err, "getting id for pair %q, %q", access, objectType)
	}
	return resultM["access_type_id"].(int64), nil
}

// validateTargetExists validates that the target of the permission
// exists. An error is returned if not. Unless we have a controller
// target, search for grant_on as a cloud.name and a model_list.uuid.
// It must be one of those.
func validateTargetExists(
	ctx context.Context,
	tx *sqlair.TX,
	targetKey string,
) error {
	if targetKey == coredatabase.ControllerNS {
		return nil
	}

	// TODO: (hml) 6-Mar-24
	// Add application offers check here when added to DDL.
	targetExists := `
SELECT &M.found_it FROM (
    SELECT 1 AS found_it FROM cloud WHERE cloud.name = $M.grant_on
    UNION
    SELECT 1 AS found_it FROM model_list WHERE model_list.uuid = $M.grant_on
)
`
	// Validate the grant_on target exists.
	targetExistsStmt, err := sqlair.Prepare(targetExists, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	// Check that 1 row exists if the grant_on value is not ControllerNS.
	// The behavior for this check is changing in sqlair, trying to
	// account for either behavior. Old behavior: return no error and
	// a slice length of zero. New behavior: if the length is 0, return
	// ErrNoRows.
	var foundIt = []sqlair.M{}
	err = tx.Query(ctx, targetExistsStmt, sqlair.M{"grant_on": targetKey}).GetAll(&foundIt)
	if err != nil {
		return errors.Annotatef(err, "verifying %q target exists", targetKey)
	}

	if len(foundIt) == 1 {
		return nil
	}

	// Any answer other than 1 is an error. The targetKey should exist
	// as a unique identifier across the controller namespace.
	if len(foundIt) == 0 {
		return errors.Annotatef(err, "permission target %q does not exist", targetKey)
	}
	return errors.Annotatef(err, "permission target %q is not unique", targetKey)
}

func objectTag(id corepermission.ID) (result names.Tag) {
	// The id has been validated already.
	switch id.ObjectType {
	case corepermission.Cloud:
		result = names.NewCloudTag(id.Key)
	case corepermission.Controller:
		result = names.NewControllerTag(id.Key)
	case corepermission.Model:
		result = names.NewModelTag(id.Key)
	case corepermission.Offer:
		result = names.NewApplicationOfferTag(id.Key)
	}
	return
}
