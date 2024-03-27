// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
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
func (st *State) CreatePermission(ctx context.Context, newPermissionUUID uuid.UUID, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error) {
	var userAccess corepermission.UserAccess

	db, err := st.DB()
	if err != nil {
		return userAccess, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		user, err := st.findUserByName(ctx, tx, spec.User)
		if err != nil {
			return errors.Trace(err)
		}

		if err := AddUserPermission(ctx, tx, AddUserPermissionArgs{
			PermissionUUID: newPermissionUUID.String(),
			UserUUID:       user.UUID,
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
	Access         corepermission.Access
	Target         corepermission.ID
}

// AddUserPermission adds a permission for the given user on the given target.
// TODO (stickupkid): Work out if there is a better location for common
// state functions.
func AddUserPermission(ctx context.Context, tx *sqlair.TX, spec AddUserPermissionArgs) error {
	// Insert a permission doc with
	// * permissionObjectAccess as permission_type_id
	// * uuid of the user (spec.User) as grant_to
	// * spec.Target.Key as grant_on
	newPermission := `
INSERT INTO permission (uuid, permission_type_id, grant_on, grant_to)
VALUES ($addUserPermission.uuid, $addUserPermission.permission_type_id, $addUserPermission.grant_on, $addUserPermission.grant_to)
`
	insertPermissionStmt, err := sqlair.Prepare(newPermission, addUserPermission{})
	if err != nil {
		return errors.Trace(err)
	}

	perm := addUserPermission{
		UUID:    spec.PermissionUUID,
		GrantOn: spec.Target.Key,
		GrantTo: spec.UserUUID,
	}

	accessTypeID, err := objectAccessID(ctx, tx, spec.Access, spec.Target.ObjectType)
	if err != nil {
		return errors.Trace(err)
	}
	perm.PermissionType = accessTypeID

	if _, err = objectType(ctx, tx, spec.Target.Key); err != nil {
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

// DeletePermission removes the given subject's (user) access to the
// given target.
// If the specified subject does not exist, a usererrors.NotFound is
// returned.
// If the permission does not exist, no error is returned.
func (st *State) DeletePermission(ctx context.Context, subject string, target corepermission.ID) error {
	// TODO: is target.Key sufficient to Delete a permission?
	db, err := st.DB()
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
		userUUID, err := st.userUUIDForName(ctx, tx, subject)
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
func (st *State) UpsertPermission(ctx context.Context, args permission.UpsertPermissionArgs) error {
	return errors.NotImplementedf("UpsertPermission")
}

// ReadUserAccessForTarget returns the subject's (user) access for the
// given user on the given target.
func (st *State) ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error) {
	var userAccess corepermission.UserAccess
	db, err := st.DB()
	if err != nil {
		return userAccess, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		user, err := st.findUserByName(ctx, tx, subject)
		if err != nil {
			return errors.Trace(err)
		}
		userAccess = user.toCoreUserAccess()

		// Based on the grant to and grant from, find the permission,
		// then the access type of it.
		accessType, permissionUUID, err := st.findAccessType(ctx, tx, target.Key, user.UUID)
		if err != nil {
			return errors.Trace(err)
		}
		userAccess.Access = corepermission.Access(accessType)
		userAccess.PermissionID = permissionUUID
		userAccess.Object = objectTag(target)
		return nil
	})
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(domain.CoerceError(err))
	}
	return userAccess, nil
}

// ReadUserAccessLevelForTarget returns the subject's (user) access level
// for the given user on the given target.
func (st *State) ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error) {
	userAccessType := corepermission.NoAccess
	db, err := st.DB()
	if err != nil {
		return userAccessType, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		userUUID, err := st.userUUIDForName(ctx, tx, subject)
		if err != nil {
			return errors.Trace(err)
		}

		// Based on the grant to and grant from, find the permission,
		// then the access type of it.
		accessType, _, err := st.findAccessType(ctx, tx, target.Key, userUUID)
		if err != nil {
			return errors.Trace(err)
		}
		userAccessType = corepermission.Access(accessType)
		return nil
	})
	if err != nil {
		return userAccessType, errors.Trace(domain.CoerceError(err))
	}
	return userAccessType, nil
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// subject's (user) has for any access type.
func (st *State) ReadAllUserAccessForUser(ctx context.Context, subject string) ([]corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var (
		permissions []readUserPermission
		user        user
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		user, err = st.findUserByName(ctx, tx, subject)
		if err != nil {
			return errors.Trace(err)
		}

		userPermissions, err := st.readUsersPermissions(ctx, tx, user.UUID)
		if err != nil {
			return errors.Trace(err)
		}

		permissions, err = grantOnType(ctx, tx, userPermissions)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	userAccess := make([]corepermission.UserAccess, len(permissions))
	for i, p := range permissions {
		userAccess[i] = p.toUserAccess(user)
	}

	return userAccess, nil
}

// ReadAllUserAccessForTarget return a slice of user access for all users
// with access to the given target.
func (st *State) ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var (
		permissions []readUserPermission
		users       map[string]user
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get all permissions for target.Key
		// Get all users from the list of permissions
		// Combine data to return a slice of UserAccess.

		permissions, err = st.targetPermissions(ctx, tx, target.Key)
		if err != nil {
			return errors.Trace(err)
		}

		userUUIDs := make([]string, len(permissions))
		for i, p := range permissions {
			userUUIDs[i] = p.GrantTo
		}
		users, err = st.findUsersByUUID(ctx, tx, userUUIDs)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	userAccess := make([]corepermission.UserAccess, len(permissions))
	for i, p := range permissions {
		user, ok := users[p.GrantTo]
		if !ok {
			return userAccess, errors.Annotatef(usererrors.NotFound, "%q", p.GrantTo)
		}
		p.ObjectType = string(target.ObjectType)
		userAccess[i] = p.toUserAccess(user)
	}

	return userAccess, nil
}

// ReadAllAccessTypeForUser return a slice of user access for the subject
// (user) specified and of the given access type.
// E.G. All clouds the user has access to.
func (st *State) ReadAllAccessTypeForUser(ctx context.Context, subject string, access_type corepermission.ObjectType) ([]corepermission.UserAccess, error) {
	return nil, errors.NotImplementedf("ReadAllAccessTypeForUser")
}

// findUserByName finds the user provided exists, hasn't been removed and is not
// disabled. Return data needed to fill in corePermission.UserAccess.
func (st *State) findUserByName(
	ctx context.Context,
	tx *sqlair.TX,
	userName string,
) (user, error) {
	var result user

	getUserQuery := `
SELECT (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&user.*),
       creator.name AS &user.created_by_name
FROM   v_user_auth u
    LEFT JOIN user AS creator
    ON        u.created_by_uuid = creator.uuid
WHERE  u.removed = false AND u.name = $M.name`

	selectUserStmt, err := st.Prepare(getUserQuery, user{}, sqlair.M{})
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

// findUsersByUUID finds the user provided exists, hasn't been removed and is not
// disabled. Return data needed to fill in corePermission.UserAccess.
func (st *State) findUsersByUUID(
	ctx context.Context,
	tx *sqlair.TX,
	userUUIDs []string,
) (map[string]user, error) {
	var results []user

	getUserQuery := `
SELECT (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&user.*),
       creator.name AS &user.created_by_name
FROM   v_user_auth u
    LEFT JOIN user AS creator
    ON        u.created_by_uuid = creator.uuid
WHERE  u.removed = false AND u.uuid IN ($S[:])`

	userUUIDSlice := sqlair.S(transform.Slice(userUUIDs, func(s string) any { return any(s) }))
	selectUserStmt, err := st.Prepare(getUserQuery, sqlair.S{}, user{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing select getUser query")
	}

	err = tx.Query(ctx, selectUserStmt, userUUIDSlice).GetAll(&results)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Annotatef(usererrors.NotFound, "%q", userUUIDs)
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting user with name %q", userUUIDs)
	}
	users := make(map[string]user, len(results))
	for _, result := range results {
		if result.Disabled {
			return nil, errors.Annotatef(usererrors.AuthenticationDisabled, "%q", userUUIDs)
		}
		users[result.UUID] = result
	}
	return users, nil
}

// userUUIDForName returns the user UUID for the associated name
// if the user is active.
// Method borrowed from the user domain state.
func (st *State) userUUIDForName(
	ctx context.Context, tx *sqlair.TX, name string,
) (string, error) {
	stmt, err := st.Prepare(
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

	return inOut["uuid"].(string), nil
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

// objectType validates that the target of the permission exists and returns
// its object type. An error is returned if not. Unless we have a controller
// target, search for grant_on as a cloud.name and a model_list.uuid.
// It must be one of those.
func objectType(
	ctx context.Context,
	tx *sqlair.TX,
	targetKey string,
) (string, error) {
	if targetKey == coredatabase.ControllerNS {
		return string(corepermission.Controller), nil
	}
	// TODO: (hml) 6-Mar-24
	// Add application offers check here when added to DDL.
	targetExists := `
SELECT &M.found_it FROM (
    SELECT "cloud" AS found_it FROM cloud WHERE cloud.name = $M.grant_on
    UNION
    SELECT "model" AS found_it FROM model_list WHERE model_list.uuid = $M.grant_on
)
`
	// Validate the grant_on target exists.
	targetExistsStmt, err := sqlair.Prepare(targetExists, sqlair.M{})
	if err != nil {
		return "", errors.Trace(err)
	}

	// Check that 1 row exists if the grant_on value is not ControllerNS.
	// The behavior for this check is changing in sqlair, trying to
	// account for either behavior. Old behavior: return no error and
	// a slice length of zero. New behavior: if the length is 0, return
	// ErrNoRows.
	var foundIt = []sqlair.M{}
	err = tx.Query(ctx, targetExistsStmt, sqlair.M{"grant_on": targetKey}).GetAll(&foundIt)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Annotatef(err, "verifying %q target exists", targetKey)
	}

	if len(foundIt) == 1 {
		return foundIt[0]["found_it"].(string), nil
	}

	// Any answer other than 1 is an error. The targetKey should exist
	// as a unique identifier across the controller namespace.
	if len(foundIt) == 0 {
		return "", fmt.Errorf("%q %w", targetKey, permissionerrors.TargetInvalid)
	}
	return "", fmt.Errorf("%q %w", targetKey, permissionerrors.UniqueIdentifierIsNotUnique)
}

// objectTag returns a names.Tag for the given ID.
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

// findAccessType returns the accessType and uuid of the
// permission for the given grantOn and grantTo combination.
// There can be only one.
func (st *State) findAccessType(
	ctx context.Context,
	tx *sqlair.TX,
	grantOn, grantTo string,
) (string, string, error) {
	findAccessTypeQuery := `
SELECT type AS &readUserPermission.access_type, permission.uuid AS &readUserPermission.uuid
FROM permission_access_type
     JOIN permission ON permission_access_type.id = permission.permission_type_id
WHERE permission.grant_on = $readUserPermission.grant_on AND permission.grant_to = $readUserPermission.grant_to
`
	findAccessTypeStmt, err := st.Prepare(findAccessTypeQuery, readUserPermission{})
	if err != nil {
		return "", "", errors.Annotate(err, "preparing select findAccessType query")
	}

	input := readUserPermission{
		GrantTo: grantTo,
		GrantOn: grantOn,
	}
	result := readUserPermission{}
	err = tx.Query(ctx, findAccessTypeStmt, input).Get(&result)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return "", "", errors.Annotatef(permissionerrors.NotFound, "for %q on %q", grantTo, grantOn)
	} else if err != nil {
		return "", "", errors.Annotatef(err, "getting permission for %q on %q", grantTo, grantOn)
	}

	return result.AccessType, result.UUID, nil
}

// readUsersPermissions returns all permissions for the grantTo, a user UUID.
func (st *State) readUsersPermissions(ctx context.Context,
	tx *sqlair.TX,
	grantTo string,
) ([]readUserPermission, error) {
	query := `
SELECT (permission.uuid, permission.grant_on) AS (&readUserPermission.*),
       permission_access_type.type AS &readUserPermission.access_type
FROM permission
     JOIN permission_access_type
     ON permission_access_type.id = permission.permission_type_id
WHERE permission.grant_to = $M.grant_to
`
	// Validate the grant_on target exists.
	stmt, err := st.Prepare(query, readUserPermission{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var usersPermissions = []readUserPermission{}
	err = tx.Query(ctx, stmt, sqlair.M{"grant_to": grantTo}).GetAll(&usersPermissions)
	if err != nil {
		return nil, errors.Annotatef(err, "collecting permissions for %q", grantTo)
	}

	if len(usersPermissions) >= 1 {
		return usersPermissions, nil
	}
	return nil, errors.Annotatef(permissionerrors.NotFound, "for %q", grantTo)
}

func grantOnType(ctx context.Context,
	tx *sqlair.TX,
	permissions []readUserPermission,
) ([]readUserPermission, error) {
	for _, p := range permissions {
		// TODO: can we make objectType work on a slice of GrantOn
		// in both use cases?
		objectType, err := objectType(ctx, tx, p.GrantOn)
		if err != nil {
			return nil, errors.Annotatef(err, "finding type for %q", p.GrantOn)
		}
		p.ObjectType = objectType
	}
	return permissions, nil
}

// targetPermissions returns a slice of readUserPermission for
// every permission available for the given target specified by
// grantOn.
func (st *State) targetPermissions(ctx context.Context,
	tx *sqlair.TX,
	grantOn string,
) ([]readUserPermission, error) {
	query := `
SELECT (permission.uuid, permission.grant_on, permission.grant_to) AS (&readUserPermission.*),
       permission_access_type.type AS &readUserPermission.access_type
FROM permission
     JOIN permission_access_type
     ON permission_access_type.id = permission.permission_type_id
WHERE permission.grant_on = $M.grant_on
`
	// Validate the grant_on target exists.
	stmt, err := st.Prepare(query, readUserPermission{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var usersPermissions = []readUserPermission{}
	err = tx.Query(ctx, stmt, sqlair.M{"grant_on": grantOn}).GetAll(&usersPermissions)
	if err != nil {
		return nil, errors.Annotatef(err, "collecting permissions for %q", grantOn)
	}

	if len(usersPermissions) >= 1 {
		return usersPermissions, nil
	}
	return nil, errors.Annotatef(permissionerrors.NotFound, "for %q", grantOn)
}
