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
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/uuid"
)

// PermissionState describes retrieval and persistence methods for storage.
type PermissionState struct {
	*domain.StateBase
	logger Logger
}

// NewPermissionState returns a new state reference.
func NewPermissionState(factory coredatabase.TxnRunnerFactory, logger Logger) *PermissionState {
	return &PermissionState{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// CreatePermission gives the user access per the provided spec.
// It requires the user/target combination has not already been
// created. UserAccess is returned on success.
// If the user provided does not exist or is marked removed,
// accesserrors.PermissionNotFound is returned.
// If the user provided exists but is marked disabled,
// accesserrors.UserAuthenticationDisabled is returned.
// If a permission for the user and target key already exists,
// accesserrors.PermissionAlreadyExists is returned.
func (st *PermissionState) CreatePermission(ctx context.Context, newPermissionUUID uuid.UUID, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error) {
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
func AddUserPermission(ctx context.Context, tx *sqlair.TX, spec AddUserPermissionArgs) error {
	// Insert a permission doc with
	// * permissionObjectAccess as permission_type_id
	// * uuid of the user (spec.User) as grant_to
	// * spec.Target.Key as grant_on
	newPermission := `
INSERT INTO permission (*)
VALUES ($dbAddUserPermission.*)
`
	insertPermissionStmt, err := sqlair.Prepare(newPermission, dbAddUserPermission{})
	if err != nil {
		return errors.Trace(err)
	}

	perm := dbAddUserPermission{
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
		return errors.Annotatef(accesserrors.PermissionAlreadyExists, "%q on %q", spec.UserUUID, spec.Target.Key)
	} else if err != nil {
		return errors.Annotatef(err, "adding permission %q for %q on %q", spec.Access, spec.UserUUID, spec.Target.Key)
	}

	return nil
}

// DeletePermission removes the given subject's (user) access to the
// given target.
// If the specified subject does not exist, a accesserrors.NotFound is
// returned.
// If the permission does not exist, no error is returned.
func (st *PermissionState) DeletePermission(ctx context.Context, subject string, target corepermission.ID) error {
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
			return errors.Annotatef(accesserrors.UserNotFound, "looking up UUID for user %q", subject)
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
func (st *PermissionState) UpsertPermission(ctx context.Context, args access.UpsertPermissionArgs) error {
	return errors.NotImplementedf("UpsertPermission")
}

// ReadUserAccessForTarget returns the subject's (user) access for the
// given user on the given target.
func (st *PermissionState) ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error) {
	var userAccess corepermission.UserAccess
	db, err := st.DB()
	if err != nil {
		return userAccess, errors.Trace(err)
	}

	type input struct {
		Name    string `db:"name"`
		GrantOn string `db:"grant_on"`
	}

	readQuery := `
SELECT  (p.uuid, p.grant_on, p.grant_to, p.access_type) AS (&dbReadUserPermission.*),
        (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&dbPermissionUser.*),
        creator.name AS &dbPermissionUser.created_by_name
FROM    v_user_auth u
        LEFT JOIN user AS creator ON u.created_by_uuid = creator.uuid
        JOIN v_permission p ON u.uuid = p.grant_to
WHERE   u.name = $input.name
AND     u.disabled = false
AND     u.removed = false
AND     p.grant_on = $input.grant_on
`

	readStmt, err := st.Prepare(readQuery, dbReadUserPermission{}, dbPermissionUser{}, input{})
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	var (
		readUser dbReadUserPermission
		permUser dbPermissionUser
	)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		in := input{
			Name:    subject,
			GrantOn: target.Key,
		}
		err = tx.Query(ctx, readStmt, in).Get(&readUser, &permUser)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(accesserrors.PermissionNotFound, "for %q on %q", subject, target.Key)
		} else if err != nil {
			return errors.Annotatef(err, "getting permission for %q on %q", subject, target.Key)
		}

		return nil
	})
	if err != nil {
		return userAccess, errors.Trace(domain.CoerceError(err))
	}

	readUser.ObjectType = string(target.ObjectType)
	return readUser.toUserAccess(permUser), nil
}

// ReadUserAccessLevelForTarget returns the subject's (user) access level
// for the given user on the given target.
func (st *PermissionState) ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error) {
	userAccessType := corepermission.NoAccess
	db, err := st.DB()
	if err != nil {
		return userAccessType, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		userAccessType, err = st.userAccessLevel(ctx, tx, subject, target)
		return nil
	})
	if err != nil {
		return userAccessType, errors.Trace(domain.CoerceError(err))
	}
	return userAccessType, nil
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// subject's (user) has for any access type.
func (st *PermissionState) ReadAllUserAccessForUser(ctx context.Context, subject string) ([]corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var (
		permissions []dbReadUserPermission
		user        dbPermissionUser
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
func (st *PermissionState) ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var (
		permissions []dbReadUserPermission
		users       map[string]dbPermissionUser
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
			return userAccess, errors.Annotatef(accesserrors.UserNotFound, "%q", p.GrantTo)
		}
		p.ObjectType = string(target.ObjectType)
		userAccess[i] = p.toUserAccess(user)
	}

	return userAccess, nil
}

// ReadAllAccessForUserAndObjectType return a slice of user access for the subject
// (user) specified and of the given access type.
// E.G. All clouds the user has access to.
func (st *PermissionState) ReadAllAccessForUserAndObjectType(ctx context.Context, subject string, objectType corepermission.ObjectType) ([]corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var (
		permissions []dbReadUserPermission
		actualUser  []dbPermissionUser
	)
	var andClause string
	switch objectType {
	case corepermission.Controller:
		andClause = `AND     p.grant_on = ctrl.c`
	case corepermission.Model:
		andClause = `AND     m.uuid NOT NULL`
	case corepermission.Cloud:
		andClause = `AND     c.name NOT NULL`
	case corepermission.Offer:
		// TODO implement for offers
		return nil, errors.NotImplementedf("ReadAllAccessForUserAndObjectType for offers")
	default:
		return nil, errors.NotValidf("object type %q", objectType)
	}
	readQuery := fmt.Sprintf(`
WITH    ctrl AS (SELECT 'controller' AS c)
SELECT  (p.uuid, p.grant_on, p.grant_to, p.access_type) AS (&dbReadUserPermission.*),
        (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&dbPermissionUser.*),
        creator.name AS &dbPermissionUser.created_by_name
FROM    v_user_auth u
        LEFT JOIN user AS creator ON u.created_by_uuid = creator.uuid
        JOIN v_permission p ON u.uuid = p.grant_to
        LEFT JOIN cloud c ON p.grant_on = c.name
        LEFT JOIN model_list m on p.grant_on = m.uuid
        LEFT JOIN ctrl ON p.grant_on = ctrl.c
WHERE   u.name = $M.name
AND     u.disabled = false
AND     u.removed = false
%s
`, andClause)

	readStmt, err := st.Prepare(readQuery, dbReadUserPermission{}, dbPermissionUser{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, sqlair.M{"name": subject}).GetAll(&permissions, &actualUser)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(accesserrors.PermissionNotFound, "for %q on %q", subject, objectType)
		} else if err != nil {
			return errors.Annotatef(err, "getting permissions for %q on %q", subject, objectType)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	userAccess := make([]corepermission.UserAccess, len(permissions))
	for i, p := range permissions {
		p.ObjectType = string(objectType)
		userAccess[i] = p.toUserAccess(actualUser[i])
	}

	return userAccess, nil
}

// findUserByName finds the user provided exists, hasn't been removed and is not
// disabled. Return data needed to fill in corePermission.UserAccess.
func (st *PermissionState) findUserByName(
	ctx context.Context,
	tx *sqlair.TX,
	userName string,
) (dbPermissionUser, error) {
	var result dbPermissionUser

	getUserQuery := `
SELECT (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&dbPermissionUser.*),
       creator.name AS &dbPermissionUser.created_by_name
FROM   v_user_auth u
       LEFT JOIN user AS creator ON u.created_by_uuid = creator.uuid
WHERE  u.removed = false
       AND u.name = $M.name`

	selectUserStmt, err := st.Prepare(getUserQuery, dbPermissionUser{}, sqlair.M{})
	if err != nil {
		return result, errors.Annotate(err, "preparing select getUser query")
	}

	err = tx.Query(ctx, selectUserStmt, sqlair.M{"name": userName}).Get(&result)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return result, errors.Annotatef(accesserrors.UserNotFound, "%q", userName)
	} else if err != nil {
		return result, errors.Annotatef(err, "getting user with name %q", userName)
	}
	if result.Disabled {
		return result, errors.Annotatef(accesserrors.UserAuthenticationDisabled, "%q", userName)
	}
	return result, nil
}

// findUsersByUUID finds the user provided exists, hasn't been removed and is not
// disabled. Return data needed to fill in corePermission.UserAccess.
func (st *PermissionState) findUsersByUUID(
	ctx context.Context,
	tx *sqlair.TX,
	userUUIDs []string,
) (map[string]dbPermissionUser, error) {
	var results []dbPermissionUser

	getUserQuery := `
SELECT (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&dbPermissionUser.*),
       creator.name AS &dbPermissionUser.created_by_name
FROM   v_user_auth u
       LEFT JOIN user AS creator ON u.created_by_uuid = creator.uuid
WHERE  u.removed = false
       AND u.uuid IN ($S[:])
`

	userUUIDSlice := sqlair.S(transform.Slice(userUUIDs, func(s string) any { return any(s) }))
	selectUserStmt, err := st.Prepare(getUserQuery, sqlair.S{}, dbPermissionUser{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing select getUser query")
	}

	err = tx.Query(ctx, selectUserStmt, userUUIDSlice).GetAll(&results)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Annotatef(accesserrors.UserNotFound, "%q", userUUIDs)
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting user with name %q", userUUIDs)
	}
	users := make(map[string]dbPermissionUser, len(results))
	for _, result := range results {
		if result.Disabled {
			return nil, errors.Annotatef(accesserrors.UserAuthenticationDisabled, "%q", userUUIDs)
		}
		users[result.UUID] = result
	}
	return users, nil
}

// userUUIDForName returns the user UUID for the associated name
// if the user is active.
// Method borrowed from the user domain state.
func (st *PermissionState) userUUIDForName(
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
			return "", errors.Annotatef(accesserrors.UserNotFound, "active user %q", name)
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
SELECT at.id AS &M.access_type_id
FROM   permission_access_type at
       INNER JOIN permission_object_access oa ON oa.access_type_id = at.id
       INNER JOIN permission_object_type ot ON ot.id = oa.object_type_id
WHERE  ot.type = $M.object_type
AND    at.type = $M.access_type
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
		return "", fmt.Errorf("%q %w", targetKey, accesserrors.PermissionTargetInvalid)
	}
	return "", fmt.Errorf("%q %w", targetKey, accesserrors.UniqueIdentifierIsNotUnique)
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
func (st *PermissionState) findAccessType(
	ctx context.Context,
	tx *sqlair.TX,
	grantOn, grantTo string,
) (string, string, error) {
	findAccessTypeQuery := `
SELECT (uuid, access_type) AS (&dbReadUserPermission.*)
FROM   v_permission
WHERE  grant_on = $dbReadUserPermission.grant_on
       AND grant_to = $dbReadUserPermission.grant_to
`
	findAccessTypeStmt, err := st.Prepare(findAccessTypeQuery, dbReadUserPermission{})
	if err != nil {
		return "", "", errors.Annotate(err, "preparing select findAccessType query")
	}

	input := dbReadUserPermission{
		GrantTo: grantTo,
		GrantOn: grantOn,
	}
	result := dbReadUserPermission{}
	err = tx.Query(ctx, findAccessTypeStmt, input).Get(&result)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return "", "", errors.Annotatef(accesserrors.PermissionNotFound, "for %q on %q", grantTo, grantOn)
	} else if err != nil {
		return "", "", errors.Annotatef(err, "getting permission for %q on %q", grantTo, grantOn)
	}

	return result.AccessType, result.UUID, nil
}

// readUsersPermissions returns all permissions for the grantTo, a user UUID.
func (st *PermissionState) readUsersPermissions(ctx context.Context,
	tx *sqlair.TX,
	grantTo string,
) ([]dbReadUserPermission, error) {
	query := `
SELECT (uuid, grant_on, grant_to, access_type) AS (&dbReadUserPermission.*)
FROM   v_permission
WHERE  grant_to = $M.grant_to
`
	// Validate the grant_on target exists.
	stmt, err := st.Prepare(query, dbReadUserPermission{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var usersPermissions = []dbReadUserPermission{}
	err = tx.Query(ctx, stmt, sqlair.M{"grant_to": grantTo}).GetAll(&usersPermissions)
	if err != nil {
		return nil, errors.Annotatef(err, "collecting permissions for %q", grantTo)
	}

	if len(usersPermissions) >= 1 {
		return usersPermissions, nil
	}
	return nil, errors.Annotatef(accesserrors.PermissionNotFound, "for %q", grantTo)
}

func grantOnType(ctx context.Context,
	tx *sqlair.TX,
	permissions []dbReadUserPermission,
) ([]dbReadUserPermission, error) {
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

// targetPermissions returns a slice of dbReadUserPermission for
// every permission available for the given target specified by
// grantOn.
func (st *PermissionState) targetPermissions(ctx context.Context,
	tx *sqlair.TX,
	grantOn string,
) ([]dbReadUserPermission, error) {
	query := `
SELECT (uuid, grant_on, grant_to, access_type) AS (&dbReadUserPermission.*)
FROM   v_permission
WHERE  grant_on = $M.grant_on
`
	// Validate the grant_on target exists.
	stmt, err := st.Prepare(query, dbReadUserPermission{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var usersPermissions = []dbReadUserPermission{}
	err = tx.Query(ctx, stmt, sqlair.M{"grant_on": grantOn}).GetAll(&usersPermissions)
	if err != nil {
		return nil, errors.Annotatef(err, "collecting permissions on %q", grantOn)
	}

	if len(usersPermissions) >= 1 {
		return usersPermissions, nil
	}
	return nil, errors.Annotatef(accesserrors.PermissionNotFound, "for %q", grantOn)
}

func (st *PermissionState) userAccessLevel(ctx context.Context, tx *sqlair.TX, subject string, target corepermission.ID) (corepermission.Access, error) {
	type inputOutput struct {
		Name    string `db:"name"`
		GrantOn string `db:"grant_on"`
		Access  string `db:"access_type"`
	}

	readQuery := `
SELECT  p.access_type AS &inputOutput.access_type
FROM    v_permission p
        LEFT JOIN v_user_auth u ON u.uuid = p.grant_to
WHERE   u.name = $input.name
AND     u.disabled = false
AND     u.removed = false
AND     p.grant_on = $input.grant_on
`

	readStmt, err := st.Prepare(readQuery, inputOutput{})
	if err != nil {
		return corepermission.NoAccess, errors.Trace(err)
	}

	inOut := inputOutput{
		Name:    subject,
		GrantOn: target.Key,
	}
	err = tx.Query(ctx, readStmt, inOut).Get(&inOut)
	if err != nil {
		return corepermission.NoAccess, errors.Annotatef(err, "reading user access level for target")
	}
	return corepermission.Access(inOut.Access), nil
}
