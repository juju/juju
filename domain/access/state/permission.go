// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
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
		return corepermission.UserAccess{}, errors.Trace(err)
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
	return errors.Trace(err)
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
		apiUserUUID, err := st.authorizedOnTarget(ctx, tx, args.ApiUser, args.AccessSpec.Target.Key)
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
			if args.External == nil {
				return fmt.Errorf("internal error: external cannot be nil when adding a user")
			}
			err = AddUserWithPermission(ctx, tx, userUUID, args.Subject, "", *args.External, apiUserUUID, args.AccessSpec)
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
		return errors.Trace(err)
	}

	return nil
}

// ReadUserAccessForTarget returns the subject's (user) access for the
// given user on the given target.
// accesserrors.PermissionNotFound is returned the users permission cannot be
// found on the target.
func (st *PermissionState) ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	user := dbPermissionUser{
		Name: subject,
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
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	var baseExternalPerms dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		err = tx.Query(ctx, readStmt, user, perm).Get(&perm, &user)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(accesserrors.UserNotFound, "looking for permissions for %q on %q", subject, target.Key)
		} else if err != nil {
			return errors.Annotatef(err, "getting permission for %q on %q", subject, target.Key)
		}

		if user.External {
			baseExternalPerms, err = st.baseExternalAccessForTarget(ctx, tx, target)
			if err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	})
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	userAccess, err := st.generateUserAccess(user, perm, baseExternalPerms)
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	return userAccess, err
}

// ReadUserAccessLevelForTarget returns the subject's (user) access level
// for the given user on the given target.
func (st *PermissionState) ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error) {
	userAccess := corepermission.NoAccess
	db, err := st.DB()
	if err != nil {
		return userAccess, errors.Trace(err)
	}

	user := dbPermissionUser{
		Name: subject,
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
		return userAccess, errors.Annotatef(err, "preparing select user access level for target statement")
	}

	var baseExternalPerms dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, user, perm).Get(&user, &perm)
		if errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("%w for %q on %q", accesserrors.AccessNotFound, subject, target.Key)
		} else if err != nil {
			return errors.Annotatef(err, "reading user access level for target")
		}

		if user.External {
			baseExternalPerms, err = st.baseExternalAccessForTarget(ctx, tx, target)
			if err != nil {
				return errors.Trace(err)
			}
		}
		return err
	})
	if err != nil {
		return userAccess, errors.Trace(err)
	}

	userAccess = corepermission.Access(perm.AccessType)
	if user.External && baseExternalAccessGreater(baseExternalPerms, userAccess) {
		return corepermission.Access(baseExternalPerms.AccessType), nil
	}
	if perm.AccessType != string(corepermission.NoAccess) {
		return userAccess, nil
	}
	return corepermission.NoAccess, fmt.Errorf("%w for %q on %q", accesserrors.AccessNotFound, subject, target.Key)
}

// ReadUserAccessLevelForTargetAddingMissingUser returns the user access level for
// the given user on the given target. If the user is external and does not yet
// exist, it is created. An accesserrors.AccessNotFound error is returned if no
// access can be found for this user, and (only in the case of external users),
// the everyone@external user.
func (st *PermissionState) ReadUserAccessLevelForTargetAddingMissingUser(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error) {
	tag := names.NewUserTag(subject)
	userAccess, err := st.ReadUserAccessLevelForTarget(ctx, subject, target)
	if err == nil {
		return userAccess, nil
	} else if !(errors.Is(err, accesserrors.AccessNotFound) && !tag.IsLocal()) {
		// If there is an access not found error and the user is external,
		// continue. Otherwise, return the error.
		return userAccess, errors.Trace(err)
	}
	db, err := st.DB()
	if err != nil {
		return userAccess, errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		everyoneExternal, err := st.findUserByName(ctx, tx, corepermission.EveryoneTagName)
		if errors.Is(err, accesserrors.UserNotFound) || errors.Is(err, accesserrors.UserAuthenticationDisabled) {
			return fmt.Errorf("%w for %q on %q", accesserrors.AccessNotFound, subject, target.Key)
		}
		userUUID, err := user.NewUUID()
		if err != nil {
			return errors.Annotate(err, "generating user UUID")
		}
		err = AddUser(ctx, tx, userUUID, subject, subject, true, user.UUID(everyoneExternal.UUID))
		if err != nil {
			return errors.Annotatef(err, "adding exteranl user %q", subject)
		}
		return nil
	})
	if err != nil {
		return userAccess, errors.Trace(err)
	}
	return st.ReadUserAccessLevelForTarget(ctx, corepermission.EveryoneTagName, target)
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// subject's (user) has for any access type.
func (st *PermissionState) ReadAllUserAccessForUser(ctx context.Context, subject string) ([]corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
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

	uName := userName{Name: subject}

	queryStmt, err := st.Prepare(query, uName, dbPermissionUser{}, dbPermission{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing select all access for user query")
	}

	var externalPerms []dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt, uName).GetAll(&users, &permissions)
		if errors.Is(err, sqlair.ErrNoRows) || len(users) == 0 {
			return errors.Annotatef(accesserrors.UserNotFound, "%q", subject)
		} else if err != nil {
			return errors.Annotatef(err, "getting user with name %q", subject)
		}

		if users[0].External {
			externalPerms, err = st.baseExternalAccess(ctx, tx)
			if err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	if users[0].Disabled {
		return nil, errors.Annotatef(accesserrors.UserAuthenticationDisabled, "%q", subject)
	}

	userAccess, err := st.generateAllUserAccess(users[0], permissions, externalPerms)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(userAccess) == 0 {
		return nil, accesserrors.PermissionNotFound
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
		return nil, errors.Annotatef(err, "preparing select all user access for target statement")
	}
	var (
		users             []dbPermissionUser
		permissions       []dbPermission
		everyoneExternals []dbEveryoneExternal
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, grantOn).GetAll(&users, &permissions, &everyoneExternals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(accesserrors.PermissionNotFound, "for %s %q", target.ObjectType, target.Key)
		} else if err != nil {
			return errors.Annotatef(err, "collecting permissions on %s %q", target.ObjectType, target.Key)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(users) != len(permissions) {
		return nil, fmt.Errorf(
			"interal error: users slice (%d) length does not match permissions slice (%d)",
			len(users), len(permissions),
		)
	}
	var userAccess []corepermission.UserAccess
	for i, user := range users {
		if user.Name == corepermission.EveryoneTagName {
			continue
		}
		ua, err := st.generateUserAccess(user, permissions[i], dbPermission(everyoneExternals[i]))
		if err != nil {
			return nil, errors.Trace(err)
		}
		userAccess = append(userAccess, ua)
	}

	return userAccess, nil
}

// ReadAllAccessForUserAndObjectType return a slice of user access for the subject
// (user) specified and of the given access type.
// E.G. All clouds the user has access to.
func (st *PermissionState) ReadAllAccessForUserAndObjectType(
	ctx context.Context, subject string, objectType corepermission.ObjectType,
) ([]corepermission.UserAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
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
		// TODO implement for offers
		return nil, errors.NotImplementedf("ReadAllAccessForUserAndObjectType for offers")
	default:
		return nil, errors.NotValidf("object type %q", objectType)
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

	uName := userName{Name: subject}

	readStmt, err := st.Prepare(readQuery, uName, dbPermission{}, dbPermissionUser{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var externalPerms []dbPermission
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, uName).GetAll(&permissions, &users)
		if errors.Is(err, sqlair.ErrNoRows) || len(users) == 0 {
			return errors.Annotatef(accesserrors.PermissionNotFound, "for %q on %q", subject, objectType)
		} else if err != nil {
			return errors.Annotatef(err, "getting permissions for %q on %q", subject, objectType)
		}

		if users[0].External {
			externalPerms, err = st.baseExternalAccess(ctx, tx)
			if err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var externalPermsOnObject []dbPermission
	for _, perm := range externalPerms {
		if perm.ObjectType == string(objectType) {
			externalPermsOnObject = append(externalPermsOnObject, perm)
		}
	}
	userAccess, err := st.generateAllUserAccess(users[0], permissions, externalPermsOnObject)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(userAccess) == 0 {
		return nil, accesserrors.PermissionNotFound
	}

	return userAccess, nil
}

// AllModelAccessForCloudCredential for a given (cloud) credential key, return all
// model name and model access level combinations.
func (st *PermissionState) AllModelAccessForCloudCredential(ctx context.Context, key credential.Key) ([]access.CredentialOwnerModelAccess, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
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
		return nil, errors.Trace(err)
	}

	in := input{
		OwnerName: key.Owner,
		CloudName: key.Cloud,
		CredName:  key.Name,
	}
	var results []access.CredentialOwnerModelAccess
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, readStmt, in).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(accesserrors.PermissionNotFound, "for %q on %q", key.Owner, key.Cloud)
		} else if err != nil {
			return errors.Annotatef(err, "getting permissions for %q on %q", key.Owner, key.Cloud)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return results, nil
}

// findUserByName finds the user provided exists, hasn't been removed and is not
// disabled. Return data needed to fill in corePermission.UserAccess.
func (st *PermissionState) findUserByName(ctx context.Context, tx *sqlair.TX, name string) (dbPermissionUser, error) {
	var result dbPermissionUser

	uName := userName{Name: name}

	getUserQuery := `
SELECT (u.uuid, u.name, u.display_name, u.external, u.created_at, u.disabled) AS (&dbPermissionUser.*),
       creator.name AS &dbPermissionUser.created_by_name
FROM   v_user_auth u
       JOIN user AS creator ON u.created_by_uuid = creator.uuid
WHERE  u.removed = false
       AND u.name = $userName.name`

	selectUserStmt, err := st.Prepare(getUserQuery, dbPermissionUser{}, uName)
	if err != nil {
		return result, errors.Annotate(err, "preparing select getUser query")
	}
	err = tx.Query(ctx, selectUserStmt, uName).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return result, errors.Annotatef(accesserrors.UserNotFound, "%q", name)
	} else if err != nil {
		return result, errors.Annotatef(err, "getting user with name %q", name)
	}
	if result.Disabled {
		return result, errors.Annotatef(accesserrors.UserAuthenticationDisabled, "%q", name)
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
	if errors.Is(err, sqlair.ErrNoRows) {
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
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Annotatef(accesserrors.UserNotFound, "active user %q", name)
	} else if err != nil {
		return "", errors.Annotatef(err, "getting user %q", name)
	}
	return inOut.UUID, nil
}

// authorizedOnTarget determines if the given user has admin/superuser
// permissions on a given target. If superuser, the user can do everything. If
// the user has admin permissions on the grantOn, the permission changes can be
// made. If the apiUser has permissions, return their UUID.
func (st *PermissionState) authorizedOnTarget(
	ctx context.Context, tx *sqlair.TX, apiUser string, grantOn string,
) (user.UUID, error) {
	var apiUserUUID user.UUID
	// Does the apiUser have superuser access?
	// Is permission the apiUser has on the target Admin?
	authQuery := `
SELECT  (p.grant_to, p.access_type) AS (&dbPermission.*)
FROM    v_permission p
        JOIN v_user_auth u ON u.uuid = p.grant_to
        LEFT JOIN controller c ON p.grant_on = c.uuid
WHERE   u.name = $permInOut.name
AND     (p.grant_on = $permInOut.grant_on OR p.grant_on = c.uuid)
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
	if errors.Is(err, sqlair.ErrNoRows) {
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
	inOut := permInOut{
		Name:    args.Subject,
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
		return errors.Annotatef(err, "preparing select existsing user access statement")
	}

	// Check the access level only if it exists, it may not for grant.
	err = tx.Query(ctx, readStmt, inOut).Get(&inOut)
	if errors.Is(err, sqlair.ErrNoRows) {
		newUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Annotate(err, "generating new UUID")
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
		Access: corepermission.Access(inOut.Access),
	}

	grantAccess := args.AccessSpec.Access
	if aSpec.EqualOrGreaterThan(grantAccess) {
		return fmt.Errorf("user %q already has %q %w", args.Subject, grantAccess, accesserrors.PermissionAccessGreater)
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
		return errors.Annotate(err, "preparing updatePermission query")
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

func (st *PermissionState) baseExternalAccessForTarget(ctx context.Context, tx *sqlair.TX, target corepermission.ID) (dbPermission, error) {
	user := dbPermissionUser{
		Name: corepermission.EveryoneTagName,
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
		return dbPermission{}, errors.Trace(err)
	}
	err = tx.Query(ctx, readStmt, user, perm).Get(&perm)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return dbPermission{}, errors.Annotatef(err, "getting external user permission on %q", target.Key)
	}
	return perm, nil
}

func (st *PermissionState) baseExternalAccess(ctx context.Context, tx *sqlair.TX) ([]dbPermission, error) {
	user := dbPermissionUser{
		Name: corepermission.EveryoneTagName,
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
		return nil, errors.Trace(err)
	}
	err = tx.Query(ctx, readStmt, user).GetAll(&perms)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Annotate(err, "getting external user permission")
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
			return everyoneExternal.toUserAccess(user), nil
		}
	}
	if perm.AccessType != "" {
		return perm.toUserAccess(user), nil
	}
	return corepermission.UserAccess{}, fmt.Errorf("%w for %q on %q", accesserrors.PermissionNotFound, user.Name, perm.GrantOn)
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
				return nil, errors.Trace(err)
			}
		}
	} else {
		for _, p := range userPerms {
			if p.AccessType != string(corepermission.NoAccess) {
				userAccess = append(userAccess, p.toUserAccess(user))
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
