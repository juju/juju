// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// PermissionState describes retrieval and persistence methods for storage.
type PermissionState struct {
	*domain.StateBase
	logger logger.Logger
}

// NewPermissionState returns a new state reference.
func NewPermissionState(factory coredatabase.TxnRunnerFactory, logger logger.Logger) *PermissionState {
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

	db, err := st.DB(ctx)
	if err != nil {
		return userAccess, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		user, err := st.findUserByName(ctx, tx, spec.User)
		if err != nil {
			return errors.Capture(err)
		}

		if user.Disabled {
			return errors.Errorf("%w: %q", accesserrors.UserAuthenticationDisabled, user.Name)
		}

		if err := AddUserPermission(ctx, tx, AddUserPermissionArgs{
			PermissionUUID: newPermissionUUID.String(),
			UserUUID:       user.UUID,
			Access:         spec.Access,
			Target:         spec.Target,
		}); err != nil {
			return errors.Capture(err)
		}

		userAccess, err = user.toCoreUserAccess()
		return errors.Capture(err)
	})
	if err != nil {
		return corepermission.UserAccess{}, errors.Capture(err)
	}

	userAccess.Access = spec.Access
	userAccess.PermissionID = newPermissionUUID.String()
	userAccess.Object = spec.Target
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
		return errors.Errorf("%q for %q %w ", spec.Access, spec.Target.Key, accesserrors.PermissionAccessInvalid)
	}
	// Validate the target exists.
	if err := targetExists(ctx, tx, spec.Target); err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}
	return nil
}

// DeletePermission removes the given subject's (user) access to the
// given target.
// If the specified subject does not exist, an [accesserrors.NotFound] is
// returned.
// If the permission does not exist, no error is returned.
func (st *PermissionState) DeletePermission(ctx context.Context, subject user.Name, target corepermission.ID) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.deletePermission(ctx, tx, subject, target)
		if err != nil {
			return errors.Errorf("delete permission: %w", err)
		}
		return nil
	})
	return errors.Capture(err)
}

// UpdatePermission updates the permission on the target for the given subject
// (user). If the subject is an external user, and they do not exist, they are
// created. Access can be granted or revoked. Revoking Read access will delete
// the permission.
// [accesserrors.UserNotFound] is returned if the user is local and does not
// exist in the users table.
// [accesserrors.PermissionAccessGreater] is returned if the user is being
// granted an access level greater or equal to what they already have.
func (st *PermissionState) UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		subjectUUID, err := st.userUUID(ctx, tx, args.Subject)
		if errors.Is(err, accesserrors.UserNotFound) && !args.Subject.IsLocal() {
			subjectUUID, err = st.addExternalUser(ctx, tx, args.Subject)
		}
		if err != nil {
			return errors.Capture(err)
		}

		switch args.Change {
		case corepermission.Grant:
			return errors.Capture(st.grantPermission(ctx, tx, subjectUUID, args))
		case corepermission.Revoke:
			return errors.Capture(st.revokePermission(ctx, tx, args))
		default:
			return errors.Errorf("change type %q %w", args.Change, coreerrors.NotValid)
		}
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// ReadUserAccessForTarget returns the subject's (user) access for the
// given user on the given target.
// accesserrors.PermissionNotFound is returned the users permission cannot be
// found on the target.
func (st *PermissionState) ReadUserAccessForTarget(ctx context.Context, subject user.Name, target corepermission.ID) (corepermission.UserAccess, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return corepermission.UserAccess{}, errors.Capture(err)
	}

	user := dbPermissionUser{
		Name: subject.Name(),
	}
	perm := dbPermission{
		GrantOn: target.Key,
	}

	readQuery := `
SELECT  (u.uuid, u.name, u.display_name, u.external, u.created_at, u.disabled) AS (&dbPermissionUser.*),
        creator.name AS &dbPermissionUser.created_by_name,
        (p.*) AS (&dbPermission.*)
FROM    v_user_auth u
        JOIN user AS creator ON u.created_by_uuid = creator.uuid
        LEFT JOIN v_permission p ON u.uuid = p.grant_to AND p.grant_on = $dbPermission.grant_on 
WHERE   u.name = $dbPermissionUser.name
AND     u.disabled = false
AND     u.removed = false
`

	readStmt, err := st.Prepare(readQuery, user, perm)
	if err != nil {
		return corepermission.UserAccess{}, errors.Capture(err)
	}

	var baseExternalPerms dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		err = tx.Query(ctx, readStmt, user, perm).Get(&perm, &user)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("looking for permissions for %q on %q: %w", subject, target.Key, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Errorf("getting permission for %q on %q: %w", subject.Name(), target.Key, err)
		}

		if user.External {
			baseExternalPerms, err = st.baseExternalAccessForTarget(ctx, tx, target)
			if err != nil {
				return errors.Capture(err)
			}
		}

		return nil
	})
	if err != nil {
		return corepermission.UserAccess{}, errors.Capture(err)
	}

	userAccess, err := st.generateUserAccess(user, perm, baseExternalPerms)
	if err != nil {
		return corepermission.UserAccess{}, errors.Errorf("for %q on %s %q: %w", subject, target.ObjectType, target.Key, err)
	}

	return userAccess, err
}

// ReadUserAccessLevelForTarget returns the subject's (user) access level
// for the given user on the given target.
// If the access level of a user cannot be found then
// accesserrors.AccessNotFound is returned.
func (st *PermissionState) ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target corepermission.ID) (corepermission.Access, error) {
	userAccess := corepermission.NoAccess
	db, err := st.DB(ctx)
	if err != nil {
		return userAccess, errors.Capture(err)
	}

	user := dbPermissionUser{
		Name: subject.Name(),
	}
	perm := dbPermission{
		GrantOn: target.Key,
	}

	readQuery := `
SELECT  (u.external) AS (&dbPermissionUser.*),
        (p.access_type, p.uuid) AS (&dbPermission.*)
FROM    v_user_auth u
        LEFT JOIN v_permission p ON u.uuid = p.grant_to AND p.grant_on = $dbPermission.grant_on
WHERE   u.name = $dbPermissionUser.name
AND     u.disabled = false
AND     u.removed = false
`

	readStmt, err := st.Prepare(readQuery, user, perm)
	if err != nil {
		return userAccess, errors.Errorf("preparing select user access level for target statement: %w", err)
	}

	var baseExternalPerms dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, user, perm).Get(&user, &perm)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w for %q on %q", accesserrors.AccessNotFound, subject, target.Key)
		} else if err != nil {
			return errors.Errorf("reading user access level for target: %w", err)
		}

		if user.External {
			baseExternalPerms, err = st.baseExternalAccessForTarget(ctx, tx, target)
			if err != nil {
				return errors.Capture(err)
			}
		}
		return err
	})
	if err != nil {
		return userAccess, errors.Capture(err)
	}

	userAccess = corepermission.Access(perm.AccessType)
	if user.External && baseExternalAccessGreater(baseExternalPerms, userAccess) {
		return corepermission.Access(baseExternalPerms.AccessType), nil
	}
	if perm.AccessType != string(corepermission.NoAccess) {
		return userAccess, nil
	}
	return corepermission.NoAccess, errors.Errorf("%w for %q on %q", accesserrors.AccessNotFound, subject, target.Key)
}

// EnsureExternalUserIfAuthorized checks if an external user is missing from the
// database and has permissions on an object. If they do then they will be
// added. This ensures that juju has a record of external users that have
// inherited their permissions from everyone@external.
func (st *PermissionState) EnsureExternalUserIfAuthorized(
	ctx context.Context,
	subject user.Name,
	target corepermission.ID,
) error {
	if subject.IsLocal() {
		return nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		_, err := st.findUserByName(ctx, tx, subject)
		if err == nil {
			return nil
		} else if !errors.Is(err, accesserrors.UserNotFound) {
			return errors.Errorf("getting user %q", subject)
		}
		// We have a UserNotFound error. Check if everyone@external has permissions
		// on the target.
		baseExternalPerms, err := st.baseExternalAccessForTarget(ctx, tx, target)
		if err != nil {
			return errors.Errorf("getting everyone@external access: %w", err)
		}
		if corepermission.Access(baseExternalPerms.AccessType) == corepermission.NoAccess {
			return nil
		}
		_, err = st.addExternalUser(ctx, tx, subject)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("adding external user %q if missing: %w", subject, err)
	}
	return nil
}

// addExternalUser adds an external user to the database with everyone@external
// as its creator.
func (st *PermissionState) addExternalUser(ctx context.Context, tx *sqlair.TX, subject user.Name) (user.UUID, error) {
	// Get the UUID of everyone@external to use as the creator.
	everyoneExternal, err := st.findUserByName(ctx, tx, corepermission.EveryoneUserName)
	if errors.Is(err, accesserrors.UserNotFound) || errors.Is(err, accesserrors.UserAuthenticationDisabled) {
		return "", errors.Errorf("%q (should be added on bootstrap): %w", corepermission.EveryoneUserName, accesserrors.UserNotFound)
	}
	userUUID, err := user.NewUUID()
	if err != nil {
		return "", errors.Errorf("generating user UUID: %w", err)
	}
	err = AddUser(ctx, tx, userUUID, subject, subject.Name(), true, user.UUID(everyoneExternal.UUID))
	if err != nil {
		return "", errors.Errorf("adding exteranl user %q: %w", subject, err)
	}
	return userUUID, nil
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// subject's (user) has for any access type.
func (st *PermissionState) ReadAllUserAccessForUser(ctx context.Context, subject user.Name) ([]corepermission.UserAccess, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var (
		users       []dbPermissionUser
		permissions []dbPermission
	)
	query := `
SELECT (u.uuid, u.name, u.display_name, u.external, u.created_at, u.disabled) AS (&dbPermissionUser.*),
       creator.name AS &dbPermissionUser.created_by_name,
       (p.*) AS (&dbPermission.*)
FROM   v_user_auth u
       JOIN user AS creator ON u.created_by_uuid = creator.uuid
       LEFT JOIN v_permission p ON u.uuid = p.grant_to
WHERE  u.removed = false
       AND u.name = $userName.name
`

	uName := userName{Name: subject.Name()}

	queryStmt, err := st.Prepare(query, uName, dbPermissionUser{}, dbPermission{})
	if err != nil {
		return nil, errors.Errorf("preparing select all access for user query: %w", err)
	}

	var externalPerms []dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt, uName).GetAll(&users, &permissions)
		if errors.Is(err, sqlair.ErrNoRows) || len(users) == 0 {
			return errors.Errorf("%q: %w", subject, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Errorf("getting user with name %q: %w", subject, err)
		}

		if users[0].External {
			externalPerms, err = st.baseExternalAccess(ctx, tx)
			if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if users[0].Disabled {
		return nil, errors.Errorf("%q: %w", subject, accesserrors.UserAuthenticationDisabled)
	}

	userAccess, err := st.generateAllUserAccess(users[0], permissions, externalPerms)
	if err != nil {
		return nil, errors.Errorf("getting permissions for user %q: %w", subject, err)
	}

	if len(userAccess) == 0 {
		return nil, accesserrors.PermissionNotFound
	}

	return userAccess, nil
}

// ReadAllUserAccessForTarget return a slice of user access for all users
// with access to the given target.
// An [accesserrors.PermissionNotFound] error is returned if no permissions can
// be found on the target.
func (st *PermissionState) ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	grantOn := dbPermission{GrantOn: target.Key}

	query := `
SELECT  (u.uuid, u.name, u.display_name, u.external, u.created_at, u.disabled) AS (&dbPermissionUser.*),
        creator.name AS &dbPermissionUser.created_by_name,
        (p.*) AS (&dbPermission.*),
        (ee.*) AS (&dbEveryoneExternal.*)
FROM    v_user_auth u
        JOIN user AS creator ON u.created_by_uuid = creator.uuid
        LEFT JOIN v_permission p ON u.uuid = p.grant_to AND p.grant_on = $dbPermission.grant_on 
        LEFT JOIN v_everyone_external ee ON ee.grant_on = $dbPermission.grant_on
WHERE   u.disabled = false
AND     u.removed = false
AND     (p.uuid IS NOT NULL) OR (u.external AND ee.uuid IS NOT NULL)
`
	stmt, err := st.Prepare(query, grantOn, dbPermissionUser{}, dbEveryoneExternal{})
	if err != nil {
		return nil, errors.Errorf("preparing select all user access for target statement: %w", err)
	}
	var (
		users             []dbPermissionUser
		permissions       []dbPermission
		everyoneExternals []dbEveryoneExternal
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, grantOn).GetAll(&users, &permissions, &everyoneExternals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("for %s %q: %w", target.ObjectType, target.Key, accesserrors.PermissionNotFound)
		} else if err != nil {
			return errors.Errorf("collecting permissions on %s %q: %w", target.ObjectType, target.Key, err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(users) != len(permissions) {
		return nil, errors.Errorf(
			"interal error: users slice (%d) length does not match permissions slice (%d)",
			len(users), len(permissions))

	}
	var userAccess []corepermission.UserAccess
	for i, user := range users {
		if user.Name == corepermission.EveryoneUserName.Name() {
			continue
		}
		ua, err := st.generateUserAccess(user, permissions[i], dbPermission(everyoneExternals[i]))
		if err != nil {
			return nil, errors.Errorf("for %q on %s %q: %w", user.Name, target.ObjectType, target.Key, err)
		}
		userAccess = append(userAccess, ua)
	}

	return userAccess, nil
}

// ReadAllAccessForUserAndObjectType return a slice of user access for the subject
// (user) specified and of the given access type.
// E.G. All clouds the user has access to.
func (st *PermissionState) ReadAllAccessForUserAndObjectType(
	ctx context.Context, subject user.Name, objectType corepermission.ObjectType,
) ([]corepermission.UserAccess, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var (
		permissions []dbPermission
		users       []dbPermissionUser
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
		view = "v_permission_offer"
	default:
		return nil, errors.Errorf("object type %q %w", objectType, coreerrors.NotValid)
	}
	readQuery := fmt.Sprintf(`
SELECT (p.*) AS (&dbPermission.*),
       (u.uuid, u.name, u.display_name, u.external, u.created_at, u.disabled) AS (&dbPermissionUser.*),
       creator.name AS &dbPermissionUser.created_by_name
FROM   v_user_auth u
       JOIN user AS creator ON u.created_by_uuid = creator.uuid
       LEFT JOIN %s p ON u.uuid = p.grant_to
WHERE  u.name = $userName.name
AND    u.disabled = false
AND    u.removed = false
`, view)

	uName := userName{Name: subject.Name()}

	readStmt, err := st.Prepare(readQuery, uName, dbPermission{}, dbPermissionUser{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var externalPerms []dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, uName).GetAll(&permissions, &users)
		if errors.Is(err, sqlair.ErrNoRows) || len(users) == 0 {
			return errors.Errorf("for %q on %q: %w", subject.Name(), objectType, accesserrors.PermissionNotFound)
		} else if err != nil {
			return errors.Errorf("getting permissions for %q on %q: %w", subject.Name(), objectType, err)
		}

		if users[0].External {
			externalPerms, err = st.baseExternalAccess(ctx, tx)
			if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var externalPermsOnObject []dbPermission
	for _, perm := range externalPerms {
		if perm.ObjectType == string(objectType) {
			externalPermsOnObject = append(externalPermsOnObject, perm)
		}
	}
	userAccess, err := st.generateAllUserAccess(users[0], permissions, externalPermsOnObject)
	if err != nil {
		return nil, errors.Errorf("getting permissions for %q on %q: %w", subject.Name(), objectType, err)
	}

	if len(userAccess) == 0 {
		return nil, accesserrors.PermissionNotFound
	}

	return userAccess, nil
}

// AllModelAccessForCloudCredential for a given (cloud) credential key, return all
// model name and model access level combinations.
func (st *PermissionState) AllModelAccessForCloudCredential(ctx context.Context, key credential.Key) ([]access.CredentialOwnerModelAccess, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type input struct {
		OwnerName string `db:"owner_name"`
		CloudName string `db:"cloud_name"`
		CredName  string `db:"cred_name"`
	}

	query := `
SELECT m.name AS &CredentialOwnerModelAccess.model_name, 
       p.access_type AS &CredentialOwnerModelAccess.access_type
FROM   v_model m
       JOIN v_permission AS p ON m.uuid = p.grant_on
WHERE  m.cloud_credential_owner_name = $input.owner_name 
AND    m.cloud_credential_cloud_name = $input.cloud_name
AND    m.cloud_credential_name = $input.cred_name
`
	readStmt, err := st.Prepare(query, input{}, access.CredentialOwnerModelAccess{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	in := input{
		OwnerName: key.Owner.Name(),
		CloudName: key.Cloud,
		CredName:  key.Name,
	}
	var results []access.CredentialOwnerModelAccess
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, in).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("for %q on %q: %w", key.Owner, key.Cloud, accesserrors.PermissionNotFound)
		} else if err != nil {
			return errors.Errorf("getting permissions for %q on %q: %w", key.Owner, key.Cloud, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return results, nil
}

// findUserByName finds the user provided exists, hasn't been removed and is not
// disabled. Return data needed to fill in corePermission.UserAccess.
func (st *PermissionState) findUserByName(ctx context.Context, tx *sqlair.TX, name user.Name) (dbPermissionUser, error) {
	var result dbPermissionUser

	uName := userName{Name: name.Name()}

	getUserQuery := `
SELECT (u.uuid, u.name, u.display_name, u.external, u.created_at, u.disabled) AS (&dbPermissionUser.*),
       creator.name AS &dbPermissionUser.created_by_name
FROM   v_user_auth u
       JOIN user AS creator ON u.created_by_uuid = creator.uuid
WHERE  u.removed = false
       AND u.name = $userName.name`

	selectUserStmt, err := st.Prepare(getUserQuery, dbPermissionUser{}, uName)
	if err != nil {
		return result, errors.Errorf("preparing select getUser query: %w", err)
	}
	err = tx.Query(ctx, selectUserStmt, uName).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return result, errors.Errorf("%q: %w", name, accesserrors.UserNotFound)
	} else if err != nil {
		return result, errors.Errorf("getting user with name %q: %w", name, err)
	}
	if result.Disabled {
		return result, errors.Errorf("%q: %w", name, accesserrors.UserAuthenticationDisabled)
	}
	return result, nil
}

// targetExists returns an error if the target does not exist in neither the
// cloud nor model tables and is not a controller.
func targetExists(ctx context.Context, tx *sqlair.TX, target corepermission.ID) error {
	var targetExists string
	switch target.ObjectType {
	case coredatabase.ControllerNS:
		targetExists = `
SELECT  uuid AS &M.found
FROM    controller
WHERE   uuid = $M.grant_on
`
	case corepermission.Model:
		targetExists = `
SELECT  uuid AS &M.found
FROM    model
WHERE   uuid = $M.grant_on
`
	case corepermission.Cloud:
		targetExists = `
SELECT  name AS &M.found
FROM    cloud
WHERE   name = $M.grant_on
`
	case corepermission.Offer:
		return nil
	default:
		return errors.Errorf("object type %q %w", target.ObjectType, coreerrors.NotValid)
	}

	targetExistsStmt, err := sqlair.Prepare(targetExists, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}

	m := sqlair.M{}
	err = tx.Query(ctx, targetExistsStmt, sqlair.M{"grant_on": target.Key}).Get(&m)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("%q %w", target, accesserrors.PermissionTargetInvalid)
	} else if err != nil {
		return errors.Errorf("verifying %q target exists: %w", target, err)
	}
	return nil
}

// userUUID returns the UUID for the associated name
// if the user is active.
func (st *PermissionState) userUUID(
	ctx context.Context, tx *sqlair.TX, name user.Name,
) (user.UUID, error) {
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
		return "", errors.Errorf("preparing user exist statement: %w", err)
	}
	inOut := inputOut{Name: name.Name()}

	err = tx.Query(ctx, stmt, inOut).Get(&inOut)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("active user %q: %w", name, accesserrors.UserNotFound)
	} else if err != nil {
		return "", errors.Errorf("getting user %q: %w", name, err)
	}
	return user.UUID(inOut.UUID), nil
}

func (st *PermissionState) grantPermission(ctx context.Context, tx *sqlair.TX, subjectUUID user.UUID, args access.UpdatePermissionArgs) error {
	inOut := permInOut{
		Name:    args.Subject.Name(),
		GrantOn: args.AccessSpec.Target.Key,
	}

	readQuery := `
SELECT  p.access_type AS &permInOut.*
FROM    v_permission p
        LEFT JOIN v_user_auth u ON u.uuid = p.grant_to
WHERE   u.name = $permInOut.name
AND     u.disabled = false
AND     u.removed = false
AND     p.grant_on = $permInOut.grant_on
`
	readStmt, err := st.Prepare(readQuery, inOut)
	if err != nil {
		return errors.Errorf("preparing select existsing user access statement: %w", err)
	}

	// Check the access level only if it exists, it may not for grant.
	err = tx.Query(ctx, readStmt, inOut).Get(&inOut)
	if errors.Is(err, sqlair.ErrNoRows) {
		newUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Errorf("generating new UUID: %w", err)
		}
		spec := args.AccessSpec
		perm := dbPermission{
			UUID:       newUUID.String(),
			GrantOn:    spec.Target.Key,
			GrantTo:    subjectUUID.String(),
			AccessType: string(spec.Access),
			ObjectType: string(spec.Target.ObjectType),
		}
		err = insertPermission(ctx, tx, perm)
		return errors.Capture(err)
	} else if err != nil {
		return errors.Errorf("getting current access for grant: %w", err)
	}

	aSpec := corepermission.AccessSpec{
		Target: args.AccessSpec.Target,
		Access: corepermission.Access(inOut.Access),
	}

	grantAccess := args.AccessSpec.Access
	if aSpec.EqualOrGreaterThan(grantAccess) {
		return errors.Errorf("user %q already has %q %w", args.Subject, grantAccess, accesserrors.PermissionAccessGreater)
	}
	if err := st.updatePermission(ctx, tx, args.Subject.Name(), args.AccessSpec.Target.Key, grantAccess.String()); err != nil {
		return errors.Errorf("updating current access during grant: %w", err)
	}
	return nil
}

func (st *PermissionState) revokePermission(ctx context.Context, tx *sqlair.TX, args access.UpdatePermissionArgs) error {
	newAccess := args.AccessSpec.RevokeAccess()
	if newAccess == corepermission.NoAccess {
		err := st.deletePermission(ctx, tx, args.Subject, args.AccessSpec.Target)
		if err != nil {
			return errors.Errorf("revoking %q: %w", args.AccessSpec.Access, err)
		}
		return nil
	}
	if err := st.updatePermission(ctx, tx, args.Subject.Name(), args.AccessSpec.Target.Key, newAccess.String()); err != nil {
		return errors.Errorf("updating current access during revoke: %w", err)
	}
	return nil
}

func (st *PermissionState) deletePermission(ctx context.Context, tx *sqlair.TX, subject user.Name, target corepermission.ID) error {
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
	deletePermissionStmt, err := st.Prepare(deletePermission, permInOut{})
	if err != nil {
		return errors.Capture(err)
	}

	in := permInOut{
		Name:    subject.Name(),
		GrantOn: target.Key,
	}
	err = tx.Query(ctx, deletePermissionStmt, in).Run()
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("deleting permission of %q on %q: %w", subject.Name(), target.Key, err)
	}
	return nil
}

func (st *PermissionState) updatePermission(ctx context.Context, tx *sqlair.TX, subjectName, grantOn, access string) error {
	updateQuery := `
UPDATE permission
SET    access_type_id = (
           SELECT id
           FROM   permission_access_type
           WHERE  type = $permInOut.access_type
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
		return errors.Errorf("preparing updatePermission query: %w", err)
	}

	in := permInOut{
		Name:    subjectName,
		GrantOn: grantOn,
		Access:  access,
	}
	if err := tx.Query(ctx, updateQueryStmt, in).Run(); err != nil {
		return errors.Errorf("updating access on %q for %q to %q: %w", grantOn, subjectName, access, err)
	}
	return nil
}

func (st *PermissionState) baseExternalAccessForTarget(ctx context.Context, tx *sqlair.TX, target corepermission.ID) (dbPermission, error) {
	user := dbPermissionUser{
		Name: corepermission.EveryoneUserName.Name(),
	}
	perm := dbPermission{
		GrantOn: target.Key,
	}
	readQuery := `
SELECT  (p.*) AS (&dbPermission.*)
FROM    v_user_auth u
        JOIN v_permission p ON u.uuid = p.grant_to AND p.grant_on = $dbPermission.grant_on 
WHERE   u.name = $dbPermissionUser.name
AND     u.disabled = false
AND     u.removed = false
	`
	readStmt, err := st.Prepare(readQuery, user, perm)
	if err != nil {
		return dbPermission{}, errors.Capture(err)
	}
	err = tx.Query(ctx, readStmt, user, perm).Get(&perm)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return dbPermission{}, errors.Errorf("getting external user permission on %q: %w", target.Key, err)
	}
	return perm, nil
}

func (st *PermissionState) baseExternalAccess(ctx context.Context, tx *sqlair.TX) ([]dbPermission, error) {
	user := dbPermissionUser{
		Name: corepermission.EveryoneUserName.Name(),
	}
	var perms []dbPermission

	readQuery := `
SELECT  (p.*) AS (&dbPermission.*)
FROM    v_user_auth u
        JOIN v_permission p ON u.uuid = p.grant_to 
WHERE   u.name = $dbPermissionUser.name
AND     u.disabled = false
AND     u.removed = false
	`
	readStmt, err := st.Prepare(readQuery, user, dbPermission{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	err = tx.Query(ctx, readStmt, user).GetAll(&perms)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting external user permission: %w", err)
	}
	return perms, nil
}

// baseExternalAccessGreater returns true if the base external access is greater
// than the users access.
func baseExternalAccessGreater(external dbPermission, userAccess corepermission.Access) bool {
	accessSpec := corepermission.AccessSpec{
		Target: corepermission.ID{
			ObjectType: corepermission.ObjectType(external.ObjectType),
			Key:        external.GrantOn,
		},
		Access: corepermission.Access(external.AccessType),
	}
	return accessSpec.EqualOrGreaterThan(userAccess)
}

// generateUserAccess generates a UserAccess object. If the user is an external
// user, their permissions are resolved against the equivalent permissions of
// the external user.
func (st *PermissionState) generateUserAccess(user dbPermissionUser, perm dbPermission, everyoneExternal dbPermission) (corepermission.UserAccess, error) {
	if user.External && everyoneExternal.AccessType != "" {
		if baseExternalAccessGreater(everyoneExternal, corepermission.Access(perm.AccessType)) {
			return everyoneExternal.toUserAccess(user)
		}
	}
	if perm.AccessType != "" {
		return perm.toUserAccess(user)
	}
	return corepermission.UserAccess{}, accesserrors.PermissionNotFound
}

// generateAllUserAccesses takes a user, their permissions and the permissions of
// the everyone@external. It returns all the users permissions including those
// inherited from everyone@external.
func (st *PermissionState) generateAllUserAccess(user dbPermissionUser, userPerms []dbPermission, externalPerms []dbPermission) ([]corepermission.UserAccess, error) {
	var userAccess []corepermission.UserAccess
	if user.External {
		allTargets := set.NewStrings()
		targetToUserPerm := map[string]dbPermission{}
		targetToExternalPerm := map[string]dbPermission{}
		for _, perm := range userPerms {
			if perm.GrantOn != "" {
				allTargets.Add(perm.GrantOn)
				targetToUserPerm[perm.GrantOn] = perm
			}
		}
		for _, perm := range externalPerms {
			if perm.GrantOn != "" {
				allTargets.Add(perm.GrantOn)
				targetToExternalPerm[perm.GrantOn] = perm
			}
		}
		userAccess = make([]corepermission.UserAccess, len(allTargets))
		var err error
		for i, target := range allTargets.Values() {
			userPerm := targetToUserPerm[target]
			externalPerm := targetToExternalPerm[target]
			userAccess[i], err = st.generateUserAccess(user, userPerm, externalPerm)
			if err != nil {
				return nil, errors.Capture(err)
			}
		}
	} else {
		for _, p := range userPerms {
			if p.AccessType != string(corepermission.NoAccess) {
				ua, err := p.toUserAccess(user)
				if err != nil {
					return nil, errors.Capture(err)
				}
				userAccess = append(userAccess, ua)
			}
		}
	}
	return userAccess, nil
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
		return errors.Capture(err)
	}

	// No IsErrConstraintForeignKey should be seen as both foreign keys
	// have been checked.
	err = tx.Query(ctx, insertPermissionStmt, perm).Run()
	if internaldatabase.IsErrConstraintUnique(err) {
		return errors.Errorf("%q on %q: %w", perm.GrantTo, perm.GrantOn, accesserrors.PermissionAlreadyExists)
	} else if err != nil {
		return errors.Errorf("adding permission %q for %q on %q: %w", perm.AccessType, perm.GrantTo, perm.GrantOn, err)
	}
	return nil
}
