// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/auth"
	internaldatabase "github.com/juju/juju/internal/database"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// UserState represents a type for interacting with the underlying state.
type UserState struct {
	*domain.StateBase
}

// NewUserState returns a new State for interacting with the underlying state.
func NewUserState(factory database.TxnRunnerFactory) *UserState {
	return &UserState{
		StateBase: domain.NewStateBase(factory),
	}
}

// AddUser will add a new user to the database. If the user already exists,
// an error that satisfies accesserrors.AlreadyExists will be returned. If the
// creator does not exist, an error that satisfies
// accesserrors.UserCreatorUUIDNotFound will be returned.
func (st *UserState) AddUser(
	ctx context.Context,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(AddUser(ctx, tx, uuid, name, displayName, creatorUUID, permission))
	})
}

// AddUserWithPasswordHash will add a new user to the database with the
// provided password hash and salt. If the user already exists, an error that
// satisfies accesserrors.UserAlreadyExists will be returned. If the creator does
// not exist that satisfies accesserrors.UserCreatorUUIDNotFound will be returned.
func (st *UserState) AddUserWithPasswordHash(
	ctx context.Context,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
	passwordHash string,
	salt []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(AddUserWithPassword(ctx, tx, uuid, name, displayName, creatorUUID, permission, passwordHash, salt))
	})
}

// AddUserWithActivationKey will add a new user to the database with the
// provided activation key. If the user already exists an error that
// satisfies accesserrors.UserAlreadyExists will be returned. if the users creator
// does not exist an error that satisfies accesserrors.UserCreatorUUIDNotFound
// will be returned.
func (st *UserState) AddUserWithActivationKey(
	ctx context.Context,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
	activationKey []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = AddUser(ctx, tx, uuid, name, displayName, creatorUUID, permission)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(setActivationKey(ctx, tx, name, activationKey))
	})
}

// GetAllUsers will retrieve all users with authentication information
// (last login, disabled) from the database. If no users exist an empty slice
// will be returned.
func (st *UserState) GetAllUsers(ctx context.Context) ([]user.User, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Annotate(err, "getting DB access")
	}

	var usrs []user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getAllUsersQuery := `
SELECT (u.uuid, u.name, u.display_name, u.created_by_uuid, u.created_at, u.last_login, u.disabled) AS (&dbUser.*),
       creator.name AS &dbUser.created_by_name
FROM   v_user_auth u
       LEFT JOIN user AS creator 
       ON        u.created_by_uuid = creator.uuid
WHERE  u.removed = false 
`

		selectGetAllUsersStmt, err := st.Prepare(getAllUsersQuery, dbUser{}, sqlair.M{})
		if err != nil {
			return errors.Annotate(err, "preparing select getAllUsers query")
		}

		var results []dbUser
		err = tx.Query(ctx, selectGetAllUsersStmt).GetAll(&results)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotate(err, "getting query results")
		}

		for _, result := range results {
			usrs = append(usrs, result.toCoreUser())
		}

		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "getting all users")
	}

	return usrs, nil
}

// GetUser will retrieve the user with authentication information (last login,
// disabled) specified by UUID from the database. If the user does not exist an
// error that satisfies accesserrors.UserNotFound will be returned.
func (st *UserState) GetUser(ctx context.Context, uuid user.UUID) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserQuery := `
SELECT (uuid, name, display_name, created_by_uuid, created_at, last_login, disabled) AS (&dbUser.*)
FROM   v_user_auth
WHERE  uuid = $M.uuid`

		selectGetUserStmt, err := st.Prepare(getUserQuery, dbUser{}, sqlair.M{})
		if err != nil {
			return errors.Annotate(err, "preparing select getUser query")
		}

		var result dbUser
		err = tx.Query(ctx, selectGetUserStmt, sqlair.M{"uuid": uuid.String()}).Get(&result)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(accesserrors.UserNotFound, "%q", uuid)
		} else if err != nil {
			return errors.Annotatef(err, "getting user with uuid %q", uuid)
		}

		usr = result.toCoreUser()

		return nil
	})
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user with uuid %q", uuid)
	}

	return usr, nil
}

// GetUserUUIDByName will retrieve the user uuid for the user identifier by name.
// If the user does not exist an error that satisfies [accesserrors.UserNotFound] will
// be returned.
// Exported for use in credential.
func GetUserUUIDByName(ctx context.Context, tx *sqlair.TX, name string) (user.UUID, error) {
	uName := userName{Name: name}

	stmt := `
SELECT user.uuid AS &M.userUUID
FROM user
WHERE user.name = $userName.name
AND user.removed = false`

	selectUserUUIDStmt, err := sqlair.Prepare(stmt, sqlair.M{}, uName)
	if err != nil {
		return "", errors.Trace(err)
	}

	result := sqlair.M{}
	err = tx.Query(ctx, selectUserUUIDStmt, uName).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w when finding user uuid for name %q", accesserrors.UserNotFound, name)
	} else if err != nil {
		return "", fmt.Errorf("looking up user uuid for name %q: %w", name, err)
	}

	if result["userUUID"] == nil {
		return "", fmt.Errorf("retrieving user uuid for user name %q, no result provided", name)
	}

	return user.UUID(result["userUUID"].(string)), nil
}

// GetUserByName will retrieve the user with authentication information
// (last login, disabled) specified by name from the database. If the user does
// not exist an error that satisfies accesserrors.UserNotFound will be returned.
func (st *UserState) GetUserByName(ctx context.Context, name string) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	uName := userName{Name: name}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserByNameQuery := `
SELECT (uuid, name, display_name, created_by_uuid, created_at, last_login, disabled) AS (&dbUser.*)
FROM   v_user_auth
WHERE  name = $userName.name
AND    removed = false`

		selectGetUserByNameStmt, err := st.Prepare(getUserByNameQuery, dbUser{}, uName)
		if err != nil {
			return errors.Annotate(err, "preparing select getUserByName query")
		}

		var result dbUser
		err = tx.Query(ctx, selectGetUserByNameStmt, uName).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(accesserrors.UserNotFound, "%q", name)
		} else if err != nil {
			return errors.Annotatef(err, "getting user with name %q", name)
		}

		usr = result.toCoreUser()

		return nil
	})
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user with name %q", name)
	}

	return usr, nil
}

// GetUserByAuth will retrieve the user with checking authentication
// information specified by UUID and password from the database.
// If the user does not exist an error that satisfies accesserrors.UserNotFound
// will be returned, otherwise unauthorized will be returned.
func (st *UserState) GetUserByAuth(ctx context.Context, name string, password auth.Password) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	uName := userName{Name: name}

	getUserWithAuthQuery := `
SELECT (
		user.uuid, user.name, user.display_name, user.created_by_uuid, user.created_at,
		user.disabled,
		user_password.password_hash, user_password.password_salt
		) AS (&dbUser.*)
FROM   v_user_auth AS user
		LEFT JOIN user_password 
		ON        user.uuid = user_password.user_uuid
WHERE  user.name = $userName.name 
AND    removed = false
	`

	selectGetUserByAuthStmt, err := st.Prepare(getUserWithAuthQuery, dbUser{}, uName)
	if err != nil {
		return user.User{}, errors.Annotate(err, "preparing select getUserWithAuth query")
	}

	var result dbUser
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, selectGetUserByAuthStmt, uName).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(accesserrors.UserNotFound, "%q", name)
		} else if err != nil {
			return errors.Annotatef(err, "getting user with name %q", name)
		}

		return nil
	})
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user with name %q", name)
	}

	passwordHash, err := auth.HashPassword(password, result.PasswordSalt)
	if errors.Is(err, errors.NotValid) {
		// If the user has no salt, then they don't have a password correctly
		// set. In this case, we should return an unauthorized error.
		return user.User{}, errors.Annotatef(accesserrors.UserUnauthorized, "%q", name)
	} else if err != nil {
		return user.User{}, errors.Annotatef(err, "hashing password for user with name %q", name)
	} else if passwordHash != result.PasswordHash {
		return user.User{}, errors.Annotatef(accesserrors.UserUnauthorized, "%q", name)
	}

	return result.toCoreUser(), nil
}

// RemoveUser marks the user as removed. This obviates the ability of a user
// to function, but keeps the user retaining provenance, i.e. auditing.
// RemoveUser will also remove any credentials and activation codes for the
// user. If no user exists for the given user name then an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) RemoveUser(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Trace(err)
	}

	m := make(sqlair.M, 1)

	deletePassStmt, err := st.Prepare("DELETE FROM user_password WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Annotate(err, "preparing password deletion query")
	}

	deleteKeyStmt, err := st.Prepare("DELETE FROM user_activation_key WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Annotate(err, "preparing password deletion query")
	}

	setRemovedStmt, err := st.Prepare("UPDATE user SET removed = true WHERE uuid = $M.uuid", m)
	if err != nil {
		return errors.Annotate(err, "preparing password deletion query")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Trace(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, deletePassStmt, m).Run(); err != nil {
			return errors.Annotatef(err, "deleting password for %q", name)
		}

		if err := tx.Query(ctx, deleteKeyStmt, m).Run(); err != nil {
			return errors.Annotatef(err, "deleting key for %q", name)
		}

		if err := tx.Query(ctx, setRemovedStmt, m).Run(); err != nil {
			return errors.Annotatef(err, "marking %q removed", name)
		}

		return nil
	})

	return errors.Annotatef(err, "removing user %q", name)
}

// SetActivationKey removes any active passwords for the user and sets the
// activation key. If no user is found for the supplied user name an error
// is returned that satisfies accesserrors.UserNotFound.
func (st *UserState) SetActivationKey(ctx context.Context, name string, activationKey []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Trace(err)
	}

	m := make(sqlair.M, 1)

	deletePassStmt, err := st.Prepare("DELETE FROM user_password WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Annotate(err, "preparing password deletion query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Trace(err)
		}

		if err := tx.Query(ctx, deletePassStmt, sqlair.M{"uuid": uuid}).Run(); err != nil {
			return errors.Annotatef(err, "deleting password for %q", name)
		}

		return errors.Trace(setActivationKey(ctx, tx, name, activationKey))
	})
}

// GetActivationKey retrieves the activation key for the user with the supplied
// user name. If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) GetActivationKey(ctx context.Context, name string) ([]byte, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Annotate(err, "getting DB access")
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return nil, errors.Trace(err)
	}

	m := make(sqlair.M, 1)

	selectKeyStmt, err := st.Prepare(`
SELECT (*) AS (&dbActivationKey.*) FROM user_activation_key WHERE user_uuid = $M.uuid
`, m, dbActivationKey{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing activation get query")
	}

	var key dbActivationKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Trace(err)
		}

		if err := tx.Query(ctx, selectKeyStmt, sqlair.M{"uuid": uuid}).Get(&key); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.Annotatef(accesserrors.ActivationKeyNotFound, "activation key for %q", name)
			}
			return errors.Annotatef(err, "selecting activation key for %q", name)
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting activation key for %q", name)
	}
	if len(key.ActivationKey) == 0 {
		return nil, errors.Annotatef(accesserrors.ActivationKeyNotValid, "activation key for %q", name)
	}
	return []byte(key.ActivationKey), nil
}

// SetPasswordHash removes any active activation keys and sets the user
// password hash and salt. If no user is found for the supplied user name an error
// is returned that satisfies accesserrors.UserNotFound.
func (st *UserState) SetPasswordHash(ctx context.Context, name string, passwordHash string, salt []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Trace(err)
	}

	m := make(sqlair.M, 1)

	deleteKeyStmt, err := st.Prepare("DELETE FROM user_activation_key WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Annotate(err, "preparing password deletion query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Trace(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, deleteKeyStmt, m).Run(); err != nil {
			return errors.Annotatef(err, "deleting key for %q", name)
		}

		return errors.Trace(setPasswordHash(ctx, tx, name, passwordHash, salt))
	})
}

// EnableUserAuthentication will enable the user with the supplied name.
// If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) EnableUserAuthentication(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Trace(err)
	}

	m := make(sqlair.M, 1)

	q := `
INSERT INTO user_authentication (user_uuid, disabled)  
VALUES ($M.uuid, false)
ON CONFLICT(user_uuid) DO 
UPDATE SET disabled = false`

	enableUserStmt, err := st.Prepare(q, m)
	if err != nil {
		return errors.Annotate(err, "preparing enable user query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Trace(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, enableUserStmt, m).Run(); err != nil {
			return errors.Annotatef(err, "enabling user %q", name)
		}

		return nil
	})
}

// DisableUserAuthentication will disable the user with the supplied user name. If the user does
// not exist an error that satisfies accesserrors.UserNotFound will be returned.
func (st *UserState) DisableUserAuthentication(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Trace(err)
	}

	m := make(sqlair.M, 1)

	q := `
INSERT INTO user_authentication (user_uuid, disabled)  
VALUES ($M.uuid, true)
ON CONFLICT(user_uuid) DO 
UPDATE SET disabled = true`

	disableUserStmt, err := st.Prepare(q, m)
	if err != nil {
		return errors.Annotate(err, "preparing disable user query")
	}

	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Trace(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, disableUserStmt, m).Run(); err != nil {
			return errors.Annotatef(err, "disabling user %q", name)
		}

		return nil
	}))
}

// AddUserWithPassword adds a new user to the database with the
// provided password hash and salt. If the user already exists an error that
// satisfies accesserrors.UserAlreadyExists will be returned. if the creator
// does not exist that satisfies accesserrors.CreatorUUIDNotFound will be
// returned.
func AddUserWithPassword(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
	passwordHash string,
	salt []byte,
) error {
	err := AddUser(ctx, tx, uuid, name, displayName, creatorUUID, permission)
	if err != nil {
		return errors.Annotatef(err, "adding user with uuid %q", uuid)
	}

	return errors.Trace(setPasswordHash(ctx, tx, name, passwordHash, salt))
}

// UpdateLastLogin updates the last login time for the user with the supplied
// uuid. If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) UpdateLastLogin(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Trace(err)
	}

	m := make(sqlair.M, 1)

	q := `
UPDATE user_authentication
SET    last_login = datetime('now')
WHERE  user_uuid = $M.uuid`

	updateLastLoginStmt, err := st.Prepare(q, m)
	if err != nil {
		return errors.Annotate(err, "preparing update updateLastLogin query")
	}

	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Trace(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, updateLastLoginStmt, m).Run(); err != nil {
			return errors.Annotatef(err, "updating last login for %q", name)
		}

		return nil
	}))
}

// AddUser adds a new user to the database, enables the user and adds the
// given permission for the user.
// If the user already exists an error that satisfies
// accesserrors.UserAlreadyExists will be returned. If the creator does not
// exist an error that satisfies accesserrors.UserCreatorUUIDNotFound will
// be returned.
func AddUser(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUuid user.UUID,
	access permission.AccessSpec,
) error {
	permissionUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Annotate(err, "generating permission UUID")
	}

	addUserQuery := `
INSERT INTO user (uuid, name, display_name, created_by_uuid, created_at) 
VALUES      ($M.uuid, $M.name, $M.display_name, $M.created_by_uuid, $M.created_at)`

	insertAddUserStmt, err := sqlair.Prepare(addUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing add user query")
	}

	m := sqlair.M{
		"uuid":            uuid.String(),
		"name":            name,
		"display_name":    displayName,
		"created_by_uuid": creatorUuid.String(),
		"created_at":      time.Now(),
	}
	err = tx.Query(ctx, insertAddUserStmt, m).Run()
	if internaldatabase.IsErrConstraintUnique(err) {
		return errors.Annotatef(accesserrors.UserAlreadyExists, "adding user %q", name)
	} else if internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Annotatef(accesserrors.UserCreatorUUIDNotFound, "adding user %q", name)
	} else if err != nil {
		return errors.Annotatef(err, "adding user %q", name)
	}

	enableUserQuery := `
INSERT INTO user_authentication (user_uuid, disabled)
VALUES ($M.uuid, false)
`

	enableUserStmt, err := sqlair.Prepare(enableUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing enable user query")
	}

	if err := tx.Query(ctx, enableUserStmt, m).Run(); err != nil {
		return errors.Annotatef(err, "enabling user %q", name)
	}

	err = AddUserPermission(ctx, tx, AddUserPermissionArgs{
		PermissionUUID: permissionUUID.String(),
		UserUUID:       uuid.String(),
		Access:         access.Access,
		Target:         access.Target,
	})
	if err != nil {
		return errors.Annotatef(err, "adding permission for user %q", name)
	}

	return nil
}

// ensureUserAuthentication ensures that the user for uuid has their
// authentication table record and that their authentication is currently not
// disabled. If a user does not exist for the supplied user name then an error is
// returned that satisfies [accesserrors.UserNotFound]. Should the user currently have
// their authentication disable an error satisfying
// [accesserrors.UserAuthenticationDisabled] is returned.
func ensureUserAuthentication(
	ctx context.Context,
	tx *sqlair.TX,
	name string,
) error {
	defineUserAuthenticationQuery := `
INSERT INTO user_authentication (user_uuid, disabled) 
    SELECT uuid, $M.disabled                    
    FROM   user
    WHERE  name = $M.name AND removed = false
ON CONFLICT(user_uuid) DO 
UPDATE SET user_uuid = excluded.user_uuid
WHERE      disabled = false`

	insertDefineUserAuthenticationStmt, err := sqlair.Prepare(defineUserAuthenticationQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert defineUserAuthentication query")
	}

	query := tx.Query(ctx, insertDefineUserAuthenticationStmt, sqlair.M{"name": name, "disabled": false})
	err = query.Run()
	if internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Annotatef(accesserrors.UserNotFound, "%q", name)
	} else if err != nil {
		return errors.Annotatef(err, "setting authentication for user %q", name)
	}

	outcome := sqlair.Outcome{}
	if err := query.Get(&outcome); err != nil {
		return errors.Annotatef(err, "setting authentication for user %q", name)
	}

	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Annotatef(err, "determining results of setting authentication for user %q", name)
	}

	if affected == 0 {
		return errors.Annotatef(accesserrors.UserAuthenticationDisabled, "%q", name)
	}
	return nil
}

// setPasswordHash sets the password hash and salt for the user with the
// supplied uuid. If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned. If the user does not have their
// authentication enabled an error that satisfies
// accesserrors.UserAuthenticationDisabled will be returned.
func setPasswordHash(ctx context.Context, tx *sqlair.TX, name string, passwordHash string, salt []byte) error {
	err := ensureUserAuthentication(ctx, tx, name)
	if err != nil {
		return errors.Annotatef(err, "setting password hash for user %q", name)
	}

	setPasswordHashQuery := `
INSERT INTO user_password (user_uuid, password_hash, password_salt) 
    SELECT uuid, $M.password_hash, $M.password_salt
    FROM   user
    WHERE  name = $M.name 
    AND    removed = false
ON CONFLICT(user_uuid) DO UPDATE SET password_hash = excluded.password_hash, password_salt = excluded.password_salt`

	insertSetPasswordHashStmt, err := sqlair.Prepare(setPasswordHashQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert setPasswordHash query")
	}

	err = tx.Query(ctx, insertSetPasswordHashStmt, sqlair.M{
		"name":          name,
		"password_hash": passwordHash,
		"password_salt": salt},
	).Run()
	if err != nil {
		return errors.Annotatef(err, "setting password hash for user %q", name)
	}

	return nil
}

// setActivationKey sets the activation key for the user with the supplied uuid.
// If the user does not exist an error that satisfies accesserrors.UserNotFound will
// be returned. If the user does not have their authentication enabled an error
// that satisfies accesserrors.UserAuthenticationDisabled will be returned.
func setActivationKey(ctx context.Context, tx *sqlair.TX, name string, activationKey []byte) error {
	err := ensureUserAuthentication(ctx, tx, name)
	if err != nil {
		return errors.Annotatef(err, "setting activation key for user %q", name)
	}

	setActivationKeyQuery := `
INSERT INTO user_activation_key (user_uuid, activation_key)
    SELECT uuid, $M.activation_key
    FROM   user
    WHERE  name = $M.name 
    AND    removed = false
ON CONFLICT(user_uuid) DO UPDATE SET activation_key = excluded.activation_key`

	insertSetActivationKeyStmt, err := sqlair.Prepare(setActivationKeyQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert setActivationKey query")
	}

	err = tx.Query(ctx, insertSetActivationKeyStmt, sqlair.M{"name": name, "activation_key": activationKey}).Run()
	if err != nil {
		return errors.Annotatef(err, "setting activation key for user %q", name)
	}

	return nil
}

func (st *UserState) uuidForName(
	ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, name string,
) (user.UUID, error) {
	var inOut = sqlair.M{"name": name}
	err := tx.Query(ctx, stmt, inOut).Get(&inOut)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.Annotatef(accesserrors.UserNotFound, "active user %q", name)
		}
		return "", errors.Annotatef(err, "getting user %q", name)
	}

	res, _ := inOut["uuid"].(string)
	uuid := user.UUID(res)
	if err := uuid.Validate(); err != nil {
		return "", errors.Annotatef(accesserrors.UserNotFound, "valid UUID for %q", name)
	}
	return uuid, nil
}

// getActiveUUIDStmt returns a SQLair prepared statement
// for retrieving the UUID of an active user.
func (st *UserState) getActiveUUIDStmt() (*sqlair.Statement, error) {
	return st.Prepare(
		"SELECT &M.uuid FROM user WHERE name = $M.name AND IFNULL(removed, false) = false", sqlair.M{})
}
