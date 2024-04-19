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
	"github.com/juju/juju/core/user"
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

		if user.Disabled {
			return fmt.Errorf("%w: %q", accesserrors.UserAuthenticationDisabled, user.Name)
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
// Validates that the target exists and that the Access level is appropriate
// for the target before insert.
func AddUserPermission(ctx context.Context, tx *sqlair.TX, spec AddUserPermissionArgs) error {
	// Validate the access is appropriate for the target.
	if err := spec.Target.ValidateAccess(spec.Access); err != nil {
		return fmt.Errorf("%q for %q %w ", spec.Access, spec.Target.Key, accesserrors.PermissionAccessInvalid)
	}
	// Validate the target exists.
	if err := targetExists(ctx, tx, spec.Target); err != nil {
		return errors.Trace(err)
	}

	perm := dbPermission{
		UUID:       spec.PermissionUUID,
		GrantOn:    spec.Target.Key,
		GrantTo:    spec.UserUUID,
		AccessType: spec.Access.String(),
		ObjectType: spec.Target.ObjectType.String(),
	}
	err := insertPermission(ctx, tx, perm)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// DeletePermission removes the given subject's (user) access to the
// given target.
// If the specified subject does not exist, a accesserrors.NotFound is
// returned.
// If the permission does not exist, no error is returned.
func (st *PermissionState) DeletePermission(ctx context.Context, subject string, target corepermission.ID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.deletePermission(ctx, tx, subject, target)
		return errors.Annotatef(err, "delete permission")
	})
	return errors.Trace(domain.CoerceError(err))
}

// UpsertPermission updates the permission on the target for the given
// subject (user). The api user must have Superuser access or Admin access
// on the target. If a subject does not exist, it is created using the subject
// and api user. Access can be granted or revoked. Revoking Read access will
// delete the permission.
func (st *PermissionState) UpsertPermission(ctx context.Context, args access.UpdatePermissionArgs) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		apiUserUUID, err := st.upsertPermissionAuthorized(ctx, tx, args.ApiUser, args.AccessSpec.Target.Key)
		if err != nil {
			return errors.Annotatef(err, "permission creator %q", args.ApiUser)
		}

		subjectUUID, err := st.userUUID(ctx, tx, args.Subject)
		if (err != nil && !errors.Is(err, accesserrors.UserNotFound)) ||
			(errors.Is(err, accesserrors.UserNotFound) && !args.AddUser) {
			return errors.Trace(err)
		}

		switch args.Change {
		case corepermission.Grant:
			if subjectUUID != "" {
				return errors.Trace(st.grantPermission(ctx, tx, subjectUUID, args))
			}
			userUUID, err := user.NewUUID()
			if err != nil {
				return errors.Annotate(err, "generating user UUID")
			}
			err = AddUser(ctx, tx, userUUID, args.Subject, "", apiUserUUID, args.AccessSpec)
			if err != nil {
				return errors.Annotatef(err, "granting permission for %q on %q", args.Subject, args.AccessSpec.Target.Key)
			}
			return nil
		case corepermission.Revoke:
			if subjectUUID == "" {
				return errors.Trace(errors.NotValidf("change type %q with non existent user %q", args.Change, args.Subject))
			}
			return errors.Trace(st.revokePermission(ctx, tx, args))
		default:
			return errors.Trace(errors.NotValidf("change type %q", args.Change))
		}
	})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	return nil
}

// ReadUserAccessForTarget returns the subject's (user) access for the
// given user on the given target.
func (st *PermissionState) ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error) {
	var userAccess corepermission.UserAccess
	db, err := st.DB()
	if err != nil {
		return userAccess, errors.Trace(err)
	}

	readQuery := `
SELECT  (p.uuid, p.grant_on, p.grant_to, p.access_type, p.object_type) AS (&dbPermission.*),
        (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&dbPermissionUser.*),
        creator.name AS &dbPermissionUser.created_by_name
FROM    v_user_auth u
        JOIN user AS creator ON u.created_by_uuid = creator.uuid
        JOIN v_permission p ON u.uuid = p.grant_to
WHERE   u.name = $permInOut.name
AND     u.disabled = false
AND     u.removed = false
AND     p.grant_on = $permInOut.grant_on
`

	readStmt, err := st.Prepare(readQuery, dbPermission{}, dbPermissionUser{}, permInOut{})
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	var (
		readUser dbPermission
		permUser dbPermissionUser
	)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		in := permInOut{
			Name:    subject,
			GrantOn: target.Key,
		}
		err = tx.Query(ctx, readStmt, in).Get(&readUser, &permUser)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w for %q on %q", accesserrors.PermissionNotFound, subject, target.Key)
		} else if err != nil {
			return errors.Annotatef(err, "getting permission for %q on %q", subject, target.Key)
		}

		return nil
	})
	if err != nil {
		return userAccess, errors.Trace(domain.CoerceError(err))
	}

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
		return err
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
		permissions []dbPermission
		users       []dbPermissionUser
	)
	query := `
SELECT (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&dbPermissionUser.*),
       creator.name AS &dbPermissionUser.created_by_name,
       (p.uuid, p.grant_on, p.grant_to, p.access_type, p.object_type) AS (&dbPermission.*)
FROM   v_user_auth u
       JOIN user AS creator ON u.created_by_uuid = creator.uuid
       JOIN v_permission p ON u.uuid = p.grant_to
WHERE  u.removed = false
       AND u.name = $permUserName.name
`

	queryStmt, err := st.Prepare(query, dbPermissionUser{}, dbPermission{}, permUserName{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing select all access for user query")
	}

	n := permUserName{Name: subject}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt, n).GetAll(&permissions, &users)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(accesserrors.UserNotFound, "%q", subject)
		} else if err != nil {
			return errors.Annotatef(err, "getting user with name %q", subject)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	userAccess := make([]corepermission.UserAccess, len(permissions))
	for i, p := range permissions {
		if users[i].Disabled {
			return nil, errors.Annotatef(accesserrors.UserAuthenticationDisabled, "%q", subject)
		}
		userAccess[i] = p.toUserAccess(users[i])
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
		permissions []dbPermission
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
		permissions []dbPermission
		actualUser  []dbPermissionUser
	)
	var view string
	switch objectType {
	case corepermission.Controller:
		view = "v_permission_controller"
	case corepermission.Model:
		view = "v_permission_model"
	case corepermission.Cloud:
		view = "v_permission_cloud"
	case corepermission.Offer:
		// TODO implement for offers
		return nil, errors.NotImplementedf("ReadAllAccessForUserAndObjectType for offers")
	default:
		return nil, errors.NotValidf("object type %q", objectType)
	}
	readQuery := fmt.Sprintf(`
WITH    ctrl AS (SELECT 'controller' AS c)
SELECT  (p.uuid, p.grant_on, p.grant_to, p.access_type, p.object_type) AS (&dbPermission.*),
        (u.uuid, u.name, u.display_name, u.created_at, u.disabled) AS (&dbPermissionUser.*),
        creator.name AS &dbPermissionUser.created_by_name
FROM    v_user_auth u
        JOIN user AS creator ON u.created_by_uuid = creator.uuid
        JOIN %s p ON u.uuid = p.grant_to
WHERE   u.name = $permUserName.name
AND     u.disabled = false
AND     u.removed = false
`, view)

	readStmt, err := st.Prepare(readQuery, dbPermission{}, dbPermissionUser{}, permUserName{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	n := permUserName{Name: subject}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, n).GetAll(&permissions, &actualUser)
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
       JOIN user AS creator ON u.created_by_uuid = creator.uuid
WHERE  u.removed = false
       AND u.name = $permUserName.name`

	selectUserStmt, err := st.Prepare(getUserQuery, dbPermissionUser{}, permUserName{})
	if err != nil {
		return result, errors.Annotate(err, "preparing select getUser query")
	}
	n := permUserName{Name: userName}
	err = tx.Query(ctx, selectUserStmt, n).Get(&result)
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
       JOIN user AS creator ON u.created_by_uuid = creator.uuid
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

// targetExists returns an error if the target does not exist in neither the
// cloud nor model tables and is not a controller.
func targetExists(ctx context.Context, tx *sqlair.TX, target corepermission.ID) error {
	var targetExists string
	switch target.ObjectType {
	case coredatabase.ControllerNS:
		if target.Key != coredatabase.ControllerNS {
			return fmt.Errorf("%q %w", target, accesserrors.PermissionTargetInvalid)
		}
		return nil
	case corepermission.Model:
		targetExists = `
SELECT  model.uuid AS &M.found
FROM    model
WHERE   model.uuid = $M.grant_on
`
	case corepermission.Cloud:
		targetExists = `
SELECT  cloud.name AS &M.found
FROM    cloud
WHERE   cloud.name = $M.grant_on
`
	case corepermission.Offer:
		return errors.NotImplementedf("db permission support for offers")
	default:
		return errors.NotValidf("object type %q", target.ObjectType)
	}

	targetExistsStmt, err := sqlair.Prepare(targetExists, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	m := sqlair.M{}
	err = tx.Query(ctx, targetExistsStmt, sqlair.M{"grant_on": target.Key}).Get(&m)
	if err != nil && errors.Is(err, sqlair.ErrNoRows) {
		return fmt.Errorf("%q %w", target, accesserrors.PermissionTargetInvalid)
	} else if err != nil {
		return errors.Annotatef(err, "verifying %q target exists", target)
	}
	return nil
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

// userUUID returns the UUID for the associated name
// if the user is active.
func (st *PermissionState) userUUID(
	ctx context.Context, tx *sqlair.TX, name string,
) (string, error) {
	type inputOut struct {
		Name string `db:"name"`
		UUID string `db:"uuid"`
	}

	stmt, err := st.Prepare(`
SELECT  u.uuid AS &inputOut.uuid
FROM    v_user_auth u
WHERE   name = $inputOut.name
AND     u.disabled = false
AND     u.removed = false
`, inputOut{})

	if err != nil {
		return "", errors.Annotate(err, "preparing user exist statement")
	}
	inOut := inputOut{Name: name}

	err = tx.Query(ctx, stmt, inOut).Get(&inOut)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.Annotatef(accesserrors.UserNotFound, "active user %q", name)
		}
		return "", errors.Annotatef(err, "getting user %q", name)
	}
	return inOut.UUID, nil
}

// targetPermissions returns a slice of dbPermission for
// every permission available for the given target specified by
// grantOn.
func (st *PermissionState) targetPermissions(ctx context.Context,
	tx *sqlair.TX,
	grantOn string,
) ([]dbPermission, error) {
	type input struct {
		GrantOn string `db:"grant_on"`
	}
	query := `
SELECT (uuid, grant_on, grant_to, access_type, object_type) AS (&dbPermission.*)
FROM   v_permission
WHERE  grant_on = $input.grant_on
`
	// Validate the grant_on target exists.
	stmt, err := st.Prepare(query, dbPermission{}, input{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var usersPermissions = []dbPermission{}
	in := input{GrantOn: grantOn}
	err = tx.Query(ctx, stmt, in).GetAll(&usersPermissions)
	if err != nil {
		return nil, errors.Annotatef(err, "collecting permissions on %q", grantOn)
	}

	if len(usersPermissions) >= 1 {
		return usersPermissions, nil
	}
	return nil, errors.Annotatef(accesserrors.PermissionNotFound, "for %q", grantOn)
}

func (st *PermissionState) userAccessLevel(ctx context.Context, tx *sqlair.TX, subject string, target corepermission.ID) (corepermission.Access, error) {
	readQuery := `
SELECT  p.access_type AS &permInOut.access
FROM    v_permission p
        LEFT JOIN v_user_auth u ON u.uuid = p.grant_to
WHERE   u.name = $permInOut.name
AND     u.disabled = false
AND     u.removed = false
AND     p.grant_on = $permInOut.grant_on
`

	readStmt, err := st.Prepare(readQuery, permInOut{})
	if err != nil {
		return corepermission.NoAccess, errors.Trace(err)
	}

	inOut := permInOut{
		Name:    subject,
		GrantOn: target.Key,
	}
	err = tx.Query(ctx, readStmt, inOut).Get(&inOut)
	if errors.Is(err, sql.ErrNoRows) {
		return corepermission.NoAccess, fmt.Errorf("%w for %q on %q", accesserrors.AccessNotFound, subject, target.Key)
	} else if err != nil {
		return corepermission.NoAccess, errors.Annotatef(err, "reading user access level for target")
	}
	return corepermission.Access(inOut.Access), nil
}

// upsertPermissionAuthorized determines if the given user is able a user and
// create/update the given permissions. If superuser, the user can do
// everything. If the user has admin permissions on the grantOn, the
// permission changes can be made. If the apiUser has permissions, return their
// UUID.
func (st *PermissionState) upsertPermissionAuthorized(
	ctx context.Context, tx *sqlair.TX, apiUser string, grantOn string,
) (user.UUID, error) {
	var apiUserUUID user.UUID
	// Does the apiUser have superuser access?
	// Is permission the apiUser has on the target Admin?
	authQuery := `
WITH    ctrl AS (SELECT 'controller' AS c)
SELECT  (p.grant_to, p.access_type) AS (&dbPermission.*)
FROM    v_permission p
        JOIN v_user_auth u ON u.uuid = p.grant_to
        LEFT JOIN ctrl ON p.grant_on = ctrl.c
WHERE   u.name = $permInOut.name
AND     (p.grant_on = $permInOut.grant_on OR p.grant_on = ctrl.c)
AND     u.disabled = false
AND     u.removed = false
`
	authStmt, err := st.Prepare(authQuery, dbPermission{}, permInOut{})
	if err != nil {
		return apiUserUUID, errors.Trace(err)
	}
	var readPerm []dbPermission
	in := permInOut{
		GrantOn: grantOn,
		Name:    apiUser,
	}
	err = tx.Query(ctx, authStmt, in).GetAll(&readPerm)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return apiUserUUID, errors.Annotatef(accesserrors.PermissionNotValid, "on %q", grantOn)
	} else if err != nil {
		return apiUserUUID, errors.Annotatef(err, "verifying authorization of %q", grantOn)
	}
	apiUserUUID = user.UUID(readPerm[0].GrantTo)

	for _, read := range readPerm {
		if read.AccessType == corepermission.SuperuserAccess.String() ||
			read.AccessType == corepermission.AdminAccess.String() {
			return apiUserUUID, nil
		}
	}

	return apiUserUUID, accesserrors.PermissionNotValid
}

func (st *PermissionState) grantPermission(ctx context.Context, tx *sqlair.TX, subjectUUID string, args access.UpdatePermissionArgs) error {
	grantAccess := args.AccessSpec.Access
	userAccessLevel, err := st.userAccessLevel(ctx, tx, args.Subject, args.AccessSpec.Target)
	// Check the access level only if it exists, it may not for grant.
	if errors.Is(err, accesserrors.AccessNotFound) {
		newUUID, err := uuid.NewUUID()
		if err != nil {

		}
		spec := args.AccessSpec
		perm := dbPermission{
			UUID:       newUUID.String(),
			GrantOn:    spec.Target.Key,
			GrantTo:    subjectUUID,
			AccessType: string(spec.Access),
			ObjectType: string(spec.Target.ObjectType),
		}
		err = insertPermission(ctx, tx, perm)
		return errors.Trace(err)
	} else if err != nil {
		return errors.Annotatef(err, "getting current access for grant")
	}

	aSpec := corepermission.AccessSpec{
		Target: args.AccessSpec.Target,
		Access: userAccessLevel,
	}

	if aSpec.EqualOrGreaterThan(grantAccess) {
		return errors.Errorf("user %q already has %q access or greater", args.Subject, grantAccess)
	}
	if err := st.updatePermission(ctx, tx, args.Subject, args.AccessSpec.Target.Key, grantAccess.String()); err != nil {
		return errors.Annotatef(err, "updating current access during grant")
	}
	return nil
}

func (st *PermissionState) revokePermission(ctx context.Context, tx *sqlair.TX, args access.UpdatePermissionArgs) error {
	newAccess := args.AccessSpec.RevokeAccess()
	if newAccess == corepermission.NoAccess {
		err := st.deletePermission(ctx, tx, args.Subject, args.AccessSpec.Target)
		return errors.Annotatef(err, "revoking %q", args.AccessSpec.Access)
	}
	if err := st.updatePermission(ctx, tx, args.Subject, args.AccessSpec.Target.Key, newAccess.String()); err != nil {
		return errors.Annotatef(err, "updating current access during revoke")
	}
	return nil
}

func (st *PermissionState) deletePermission(ctx context.Context, tx *sqlair.TX, subject string, target corepermission.ID) error {
	// The combination of grant_to and grant_on are guaranteed to be
	// unique, thus it is all that is deleted to select the row to be
	// deleted.
	deletePermission := `
DELETE FROM permission
WHERE  grant_on = $permInOut.grant_on
AND    grant_to IN (
       SELECT uuid
       FROM   user
       WHERE  name = $permInOut.name
)
`
	deletePermissionStmt, err := sqlair.Prepare(deletePermission, permInOut{})
	if err != nil {
		return errors.Trace(err)
	}

	in := permInOut{
		Name:    subject,
		GrantOn: target.Key,
	}
	err = tx.Query(ctx, deletePermissionStmt, in).Run()
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Annotatef(err, "deleting permission of %q on %q", subject, target.Key)
	}
	return nil
}

func (st *PermissionState) updatePermission(ctx context.Context, tx *sqlair.TX, subjectName, grantOn, access string) error {
	updateQuery := `
UPDATE permission
SET    access_type_id = (
           SELECT id
           FROM   permission_access_type
           WHERE  type = $permInOut.access
       )
WHERE  grant_on = $permInOut.grant_on
AND    grant_to IN (
           SELECT uuid
           FROM   v_user_auth
           WHERE  name = $permInOut.name
           AND    removed = false
           AND    disabled = false
       )
`
	updateQueryStmt, err := st.Prepare(updateQuery, permInOut{})
	if err != nil {
		return errors.Annotate(err, "preparing update updateLastLogin query")
	}

	in := permInOut{
		Name:    subjectName,
		GrantOn: grantOn,
		Access:  access,
	}
	if err := tx.Query(ctx, updateQueryStmt, in).Run(); err != nil {
		return errors.Annotatef(err, "updating access on %q for %q to %q", grantOn, subjectName, access)
	}
	return nil
}

func insertPermission(ctx context.Context, tx *sqlair.TX, perm dbPermission) error {
	// Insert a permission doc with
	// * id of access type as access_type_id
	// * id of object type as object_type_id
	// * uuid of the user (spec.User) as grant_to
	// * spec.Target.Key as grant_on
	newPermission := `
INSERT INTO permission (uuid, access_type_id, object_type_id, grant_to, grant_on)
SELECT $dbPermission.uuid,
       at.id,
       ot.id,
       u.uuid,
       $dbPermission.grant_on
FROM   v_user_auth u,
       permission_access_type at,
       permission_object_type ot
WHERE  u.uuid = $dbPermission.grant_to
AND    u.disabled = false
AND    u.removed = false
AND    at.type = $dbPermission.access_type
AND    ot.type = $dbPermission.object_type
`

	insertPermissionStmt, err := sqlair.Prepare(newPermission, dbPermission{})
	if err != nil {
		return errors.Trace(err)
	}

	// No IsErrConstraintForeignKey should be seen as both foreign keys
	// have been checked.
	err = tx.Query(ctx, insertPermissionStmt, perm).Run()
	if internaldatabase.IsErrConstraintUnique(err) {
		return errors.Annotatef(accesserrors.PermissionAlreadyExists, "%q on %q", perm.GrantTo, perm.GrantOn)
	} else if err != nil {
		return errors.Annotatef(err, "adding permission %q for %q on %q", perm.AccessType, perm.GrantTo, perm.GrantOn)
	}
	return nil
}
