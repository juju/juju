// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/auth"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
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

// AddUser adds a new user to the database and enables the user.
// If the user already exists an error that satisfies
// [accesserrors.UserAlreadyExists] will be returned. If the creator does not
// exist an error that satisfies [accesserrors.UserCreatorUUIDNotFound] will
// be returned.
func (st *UserState) AddUser(
	ctx context.Context,
	uuid user.UUID,
	name user.Name,
	displayName string,
	external bool,
	creatorUUID user.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(AddUser(ctx, tx, uuid, name, displayName, external, creatorUUID))
	})
}

// AddUserWithPermission will add a new user and a permission to the database.
// If the user already exists, an error that satisfies
// [accesserrors.UserAlreadyExists] will be returned. If the creator does not
// exist, an error that satisfies [accesserrors.UserCreatorUUIDNotFound] will be
// returned.
func (st *UserState) AddUserWithPermission(
	ctx context.Context,
	uuid user.UUID,
	name user.Name,
	displayName string,
	external bool,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(AddUserWithPermission(ctx, tx, uuid, name, displayName, external, creatorUUID, permission))
	})
}

// AddUserWithPasswordHash will add a new user to the database with the provided
// password hash and salt. If the user already exists, an error that satisfies
// [accesserrors.UserAlreadyExists] will be returned. If the creator does not
// exist that satisfies [accesserrors.UserCreatorUUIDNotFound] will be returned.
func (st *UserState) AddUserWithPasswordHash(
	ctx context.Context,
	uuid user.UUID,
	name user.Name,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
	passwordHash string,
	salt []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(AddUserWithPassword(ctx, tx, uuid, name, displayName, creatorUUID, permission, passwordHash, salt))
	})
}

// AddUserWithActivationKey will add a new user to the database with the
// provided activation key. If the user already exists an error that satisfies
// [accesserrors.UserAlreadyExists] will be returned. if the users creator does
// not exist an error that satisfies [accesserrors.UserCreatorUUIDNotFound] will
// be returned.
func (st *UserState) AddUserWithActivationKey(
	ctx context.Context,
	uuid user.UUID,
	name user.Name,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
	activationKey []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = AddUserWithPermission(ctx, tx, uuid, name, displayName, false, creatorUUID, permission)
		if err != nil {
			return errors.Capture(err)
		}
		return errors.Capture(setActivationKey(ctx, tx, name, activationKey))
	})
}

// GetAllUsers will retrieve all users with authentication information
// (last login, disabled) from the database. If no users exist an empty slice
// will be returned.
func (st *UserState) GetAllUsers(ctx context.Context, includeDisabled bool) ([]user.User, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Errorf("getting DB access: %w", err)
	}

	selectGetAllUsersStmt, err := st.Prepare(`
SELECT (u.uuid, u.name, u.display_name, u.created_by_uuid, u.created_at, u.disabled, ull.last_login) AS (&dbUser.*),
       creator.name AS &dbUser.created_by_name
FROM   v_user_auth u
       LEFT JOIN user AS creator
       ON        u.created_by_uuid = creator.uuid
       LEFT JOIN v_user_last_login AS ull
       ON        u.uuid = ull.user_uuid
WHERE  u.removed = false
`, dbUser{})
	if err != nil {
		return nil, errors.Errorf("preparing select getAllUsers query: %w", err)
	}

	var results []dbUser
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, selectGetAllUsersStmt).GetAll(&results)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting query results: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting all users: %w", err)
	}

	var usrs []user.User
	for _, result := range results {
		if !result.Disabled || includeDisabled {
			coreUser, err := result.toCoreUser()
			if err != nil {
				return nil, errors.Capture(err)
			}
			usrs = append(usrs, coreUser)
		}
	}

	return usrs, nil
}

// GetUser will retrieve the user with authentication information specified by
// UUID from the database. If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) GetUser(ctx context.Context, uuid user.UUID) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Errorf("getting DB access: %w", err)
	}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserQuery := `
SELECT (u.uuid,
       u.name,
       u.display_name,
       u.created_by_uuid,
       u.created_at,
       u.disabled,
       ull.last_login) AS (&dbUser.*),
       creator.name AS &dbUser.created_by_name
FROM   v_user_auth u
       LEFT JOIN v_user_last_login ull ON u.uuid = ull.user_uuid
       LEFT JOIN user AS creator ON u.created_by_uuid = creator.uuid
WHERE  u.uuid = $M.uuid`

		selectGetUserStmt, err := st.Prepare(getUserQuery, dbUser{}, sqlair.M{})
		if err != nil {
			return errors.Errorf("preparing select getUser query: %w", err)
		}

		var result dbUser
		err = tx.Query(ctx, selectGetUserStmt, sqlair.M{"uuid": uuid.String()}).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("%q: %w", uuid, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Errorf("getting user with uuid %q: %w", uuid, err)
		}

		usr, err = result.toCoreUser()
		return errors.Capture(err)
	})
	if err != nil {
		return user.User{}, errors.Errorf("getting user with uuid %q: %w", uuid, err)
	}

	return usr, nil
}

// GetUserUUIDByName will retrieve the user uuid for the user identifier by name.
// If the user does not exist an error that satisfies [accesserrors.UserNotFound] will
// be returned.
// Exported for use in credential.
func GetUserUUIDByName(ctx context.Context, tx *sqlair.TX, name user.Name) (user.UUID, error) {
	uName := userName{Name: name.Name()}

	stmt := `
SELECT user.uuid AS &M.userUUID
FROM user
WHERE user.name = $userName.name
AND user.removed = false`

	selectUserUUIDStmt, err := sqlair.Prepare(stmt, sqlair.M{}, uName)
	if err != nil {
		return "", errors.Capture(err)
	}

	result := sqlair.M{}
	err = tx.Query(ctx, selectUserUUIDStmt, uName).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.Errorf("%w when finding user uuid for name %q", accesserrors.UserNotFound, name)
	} else if err != nil {
		return "", errors.Errorf("looking up user uuid for name %q: %w", name, err)
	}

	if result["userUUID"] == nil {
		return "", errors.Errorf("retrieving user uuid for user name %q, no result provided", name)
	}

	return user.UUID(result["userUUID"].(string)), nil
}

// GetUserByName will retrieve the user with authentication information
// (last login, disabled) specified by name from the database. If the user does
// not exist an error that satisfies accesserrors.UserNotFound will be returned.
func (st *UserState) GetUserByName(ctx context.Context, name user.Name) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Errorf("getting DB access: %w", err)
	}

	uName := userName{Name: name.Name()}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserByNameQuery := `
SELECT (u.uuid,
       u.name,
       u.display_name,
       u.created_by_uuid,
       u.created_at,
       u.disabled,
       ull.last_login) AS (&dbUser.*),
       creator.name AS &dbUser.created_by_name
FROM   v_user_auth u
       LEFT JOIN v_user_last_login ull ON u.uuid = ull.user_uuid
       LEFT JOIN user AS creator ON u.created_by_uuid = creator.uuid
WHERE  u.name = $userName.name
AND    u.removed = false`

		selectGetUserByNameStmt, err := st.Prepare(getUserByNameQuery, dbUser{}, uName)
		if err != nil {
			return errors.Errorf("preparing select getUserByName query: %w", err)
		}

		var result dbUser
		err = tx.Query(ctx, selectGetUserByNameStmt, uName).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("%q: %w", name, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Errorf("getting user with name %q: %w", name, err)
		}

		usr, err = result.toCoreUser()
		return errors.Capture(err)
	})
	if err != nil {
		return user.User{}, errors.Errorf("getting user with name %q: %w", name, err)
	}

	return usr, nil
}

// GetUserUUIDByName will retrieve the user UUID specified by name.
// The following errors can be expected:
// - [accesserrors.UserNotFound] when no user exists for the name.
func (st *UserState) GetUserUUIDByName(
	ctx context.Context,
	name user.Name,
) (user.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Errorf("getting DB access: %w", err)
	}

	var rval user.UUID
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		rval, err = GetUserUUIDByName(ctx, tx, name)
		return err
	})
}

// GetUserByAuth will retrieve the user with checking authentication
// information specified by UUID and password from the database.
// If the user does not exist an error that satisfies accesserrors.UserNotFound
// will be returned, otherwise unauthorized will be returned.
func (st *UserState) GetUserByAuth(ctx context.Context, name user.Name, password auth.Password) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Errorf("getting DB access: %w", err)
	}

	uName := userName{Name: name.Name()}

	getUserWithAuthQuery := `
SELECT (
       user.uuid, user.name, user.display_name, user.created_by_uuid, user.created_at,
       user.disabled,
       user_password.password_hash, user_password.password_salt
       ) AS (&dbUser.*),
       creator.name AS &dbUser.created_by_name
FROM   v_user_auth AS user
       LEFT JOIN user AS creator ON user.created_by_uuid = creator.uuid
       LEFT JOIN user_password ON user.uuid = user_password.user_uuid
WHERE  user.name = $userName.name
AND    user.removed = false
	`

	selectGetUserByAuthStmt, err := st.Prepare(getUserWithAuthQuery, dbUser{}, uName)
	if err != nil {
		return user.User{}, errors.Errorf("preparing select getUserWithAuth query: %w", err)
	}

	var result dbUser
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, selectGetUserByAuthStmt, uName).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("%q: %w", name, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Errorf("getting user with name %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return user.User{}, errors.Errorf("getting user with name %q: %w", name, err)
	}

	passwordHash, err := auth.HashPassword(password, result.PasswordSalt)
	if errors.Is(err, coreerrors.NotValid) {
		// If the user has no salt, then they don't have a password correctly
		// set. In this case, we should return an unauthorized error.
		return user.User{}, errors.Errorf("%q: %w", name, accesserrors.UserUnauthorized)
	} else if err != nil {
		return user.User{}, errors.Errorf("hashing password for user with name %q: %w", name, err)
	} else if passwordHash != result.PasswordHash {
		return user.User{}, errors.Errorf("%q: %w", name, accesserrors.UserUnauthorized)
	}

	coreUser, err := result.toCoreUser()
	return coreUser, errors.Capture(err)
}

// RemoveUser marks the user as removed. This obviates the ability of a user
// to function, but keeps the user retaining provenance, i.e. auditing.
// RemoveUser will also remove any credentials and activation codes for the
// user. If no user exists for the given user name then an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) RemoveUser(ctx context.Context, name user.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Capture(err)
	}

	m := make(sqlair.M, 1)

	deleteModelAuthorizedKeysStmt, err := st.Prepare(`
DELETE FROM model_authorized_keys
WHERE user_public_ssh_key_id IN (SELECT id
								 FROM user_public_ssh_key as upsk
								 WHERE upsk.user_uuid = $M.uuid)
	`, m)
	if err != nil {
		return errors.Errorf("preparing delete model authorized keys query for user: %w", err)
	}

	deleteUserPublicSSHKeysStmt, err := st.Prepare(
		"DELETE FROM user_public_ssh_key WHERE user_uuid = $M.uuid", m,
	)
	if err != nil {
		return errors.Errorf("preparing delete user public ssh keys: %w", err)
	}

	deletePassStmt, err := st.Prepare("DELETE FROM user_password WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Errorf("preparing password deletion query: %w", err)
	}

	deleteKeyStmt, err := st.Prepare("DELETE FROM user_activation_key WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Errorf("preparing activation key deletion query: %w", err)
	}

	setRemovedStmt, err := st.Prepare("UPDATE user SET removed = true WHERE uuid = $M.uuid", m)
	if err != nil {
		return errors.Errorf("preparing password deletion query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, deleteModelAuthorizedKeysStmt, m).Run(); err != nil {
			return errors.Errorf("deleting model authorized keys for %q: %w", name, err)
		}

		if err := tx.Query(ctx, deleteUserPublicSSHKeysStmt, m).Run(); err != nil {
			return errors.Errorf("deleting user publish ssh keys for %q: %w", name, err)
		}

		if err := tx.Query(ctx, deletePassStmt, m).Run(); err != nil {
			return errors.Errorf("deleting password for %q: %w", name, err)
		}

		if err := tx.Query(ctx, deleteKeyStmt, m).Run(); err != nil {
			return errors.Errorf("deleting key for %q: %w", name, err)
		}

		if err := tx.Query(ctx, setRemovedStmt, m).Run(); err != nil {
			return errors.Errorf("marking %q removed: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("removing user %q: %w", name, err)
	}
	return nil
}

// SetActivationKey removes any active passwords for the user and sets the
// activation key. If no user is found for the supplied user name an error
// is returned that satisfies accesserrors.UserNotFound.
func (st *UserState) SetActivationKey(ctx context.Context, name user.Name, activationKey []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Capture(err)
	}

	m := make(sqlair.M, 1)

	deletePassStmt, err := st.Prepare("DELETE FROM user_password WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Errorf("preparing password deletion query: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deletePassStmt, sqlair.M{"uuid": uuid}).Run(); err != nil {
			return errors.Errorf("deleting password for %q: %w", name, err)
		}

		return errors.Capture(setActivationKey(ctx, tx, name, activationKey))
	})
}

// GetActivationKey retrieves the activation key for the user with the supplied
// user name. If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) GetActivationKey(ctx context.Context, name user.Name) ([]byte, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return nil, errors.Capture(err)
	}

	m := make(sqlair.M, 1)

	selectKeyStmt, err := st.Prepare(`
SELECT (*) AS (&dbActivationKey.*) FROM user_activation_key WHERE user_uuid = $M.uuid
`, m, dbActivationKey{})
	if err != nil {
		return nil, errors.Errorf("preparing activation get query: %w", err)
	}

	var key dbActivationKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, selectKeyStmt, sqlair.M{"uuid": uuid}).Get(&key); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.Errorf("activation key for %q: %w", name, accesserrors.ActivationKeyNotFound)
			}
			return errors.Errorf("selecting activation key for %q: %w", name, err)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting activation key for %q: %w", name, err)
	}
	if len(key.ActivationKey) == 0 {
		return nil, errors.Errorf("activation key for %q: %w", name, accesserrors.ActivationKeyNotValid)
	}
	return []byte(key.ActivationKey), nil
}

// SetPasswordHash removes any active activation keys and sets the user
// password hash and salt. If no user is found for the supplied user name an error
// is returned that satisfies accesserrors.UserNotFound.
func (st *UserState) SetPasswordHash(ctx context.Context, name user.Name, passwordHash string, salt []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Capture(err)
	}

	m := make(sqlair.M, 1)

	deleteKeyStmt, err := st.Prepare("DELETE FROM user_activation_key WHERE user_uuid = $M.uuid", m)
	if err != nil {
		return errors.Errorf("preparing password deletion query: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, deleteKeyStmt, m).Run(); err != nil {
			return errors.Errorf("deleting key for %q: %w", name, err)
		}

		return errors.Capture(setPasswordHash(ctx, tx, name, passwordHash, salt))
	})
}

// EnableUserAuthentication will enable the user with the supplied name.
// If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned.
func (st *UserState) EnableUserAuthentication(ctx context.Context, name user.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Capture(err)
	}

	m := make(sqlair.M, 1)

	q := `
INSERT INTO user_authentication (user_uuid, disabled)
VALUES ($M.uuid, false)
ON CONFLICT(user_uuid) DO
UPDATE SET disabled = false`

	enableUserStmt, err := st.Prepare(q, m)
	if err != nil {
		return errors.Errorf("preparing enable user query: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, enableUserStmt, m).Run(); err != nil {
			return errors.Errorf("enabling user %q: %w", name, err)
		}

		return nil
	})
}

// DisableUserAuthentication will disable the user with the supplied user name. If the user does
// not exist an error that satisfies accesserrors.UserNotFound will be returned.
func (st *UserState) DisableUserAuthentication(ctx context.Context, name user.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Capture(err)
	}

	m := make(sqlair.M, 1)

	q := `
INSERT INTO user_authentication (user_uuid, disabled)
VALUES ($M.uuid, true)
ON CONFLICT(user_uuid) DO
UPDATE SET disabled = true`

	disableUserStmt, err := st.Prepare(q, m)
	if err != nil {
		return errors.Errorf("preparing disable user query: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}
		m["uuid"] = uuid

		if err := tx.Query(ctx, disableUserStmt, m).Run(); err != nil {
			return errors.Errorf("disabling user %q: %w", name, err)
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
	name user.Name,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
	passwordHash string,
	salt []byte,
) error {
	err := AddUserWithPermission(ctx, tx, uuid, name, displayName, false, creatorUUID, permission)
	if err != nil {
		return errors.Errorf("adding user with uuid %q: %w", uuid, err)
	}

	return errors.Capture(setPasswordHash(ctx, tx, name, passwordHash, salt))
}

// AddUser adds a new user to the database and enables the user.
// If the user already exists an error that satisfies
// accesserrors.UserAlreadyExists will be returned. If the creator does not
// exist an error that satisfies accesserrors.UserCreatorUUIDNotFound will
// be returned.
func AddUser(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
	name user.Name,
	displayName string,
	external bool,
	creatorUuid user.UUID,
) error {
	user := dbUser{
		UUID:        uuid.String(),
		Name:        name.Name(),
		DisplayName: displayName,
		External:    external,
		CreatorUUID: creatorUuid.String(),
		CreatedAt:   time.Now(),
	}

	addUserQuery := `
INSERT INTO user (uuid, name, display_name, external, created_by_uuid, created_at)
VALUES           ($dbUser.*)`

	insertAddUserStmt, err := sqlair.Prepare(addUserQuery, user)
	if err != nil {
		return errors.Errorf("preparing add user query: %w", err)
	}

	err = tx.Query(ctx, insertAddUserStmt, user).Run()
	if internaldatabase.IsErrConstraintUnique(err) {
		return errors.Errorf("adding user %q: %w", name, accesserrors.UserAlreadyExists)
	} else if internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("adding user %q: %w", name, accesserrors.UserCreatorUUIDNotFound)
	} else if err != nil {
		return errors.Errorf("adding user %q: %w", name, err)
	}

	enableUserQuery := `
INSERT INTO user_authentication (user_uuid, disabled)
VALUES ($dbUser.uuid, false)
`

	enableUserStmt, err := sqlair.Prepare(enableUserQuery, user)
	if err != nil {
		return errors.Errorf("preparing enable user query: %w", err)
	}

	if err := tx.Query(ctx, enableUserStmt, user).Run(); err != nil {
		return errors.Errorf("enabling user %q: %w", name, err)
	}

	return nil
}

// AddUserWithPermission adds a new user to the database, enables the user and adds the
// given permission for the user.
// If the user already exists an error that satisfies
// accesserrors.UserAlreadyExists will be returned. If the creator does not
// exist an error that satisfies accesserrors.UserCreatorUUIDNotFound will
// be returned.
func AddUserWithPermission(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
	name user.Name,
	displayName string,
	external bool,
	creatorUuid user.UUID,
	access permission.AccessSpec,
) error {
	err := AddUser(ctx, tx, uuid, name, displayName, external, creatorUuid)
	if err != nil {
		return errors.Capture(err)
	}

	permissionUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating permission UUID: %w", err)
	}
	err = AddUserPermission(ctx, tx, AddUserPermissionArgs{
		PermissionUUID: permissionUUID.String(),
		UserUUID:       uuid.String(),
		Access:         access.Access,
		Target:         access.Target,
	})
	if err != nil {
		return errors.Errorf("adding permission for user %q: %w", name, err)
	}

	return nil
}

// UpdateLastModelLogin updates the last login time for the user
// with the supplied uuid on the model with the supplied model uuid.
// The following error types are possible from this function:
// - [accesserrors.UserNameNotValid] when the username is not valid.
// - [accesserrors.UserNotFound] when the user cannot be found.
// - [modelerrors.NotFound] if no model by the given modelUUID exists.
func (st *UserState) UpdateLastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID, lastLogin time.Time) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return errors.Capture(err)
	}

	insertModelLoginStmt, err := st.Prepare(`
INSERT INTO model_last_login (model_uuid, user_uuid, time)
VALUES ($dbModelLastLogin.*)
ON CONFLICT(model_uuid, user_uuid) DO UPDATE SET
	time = excluded.time`, dbModelLastLogin{})
	if err != nil {
		return errors.Errorf("preparing insert model login query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		userUUID, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}

		mll := dbModelLastLogin{
			UserUUID:  userUUID.String(),
			ModelUUID: modelUUID.String(),
			Time:      lastLogin.Truncate(time.Second),
		}

		if err := tx.Query(ctx, insertModelLoginStmt, mll).Run(); err != nil {
			if internaldatabase.IsErrConstraintForeignKey(err) {
				// The foreign key constrain may be triggered if the user or the
				// model does not exist. However, the user must exist or the
				// uuidForName query would have failed, so it must be the model.
				return modelerrors.NotFound
			}
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("inserting last login for user %q for model %q: %w", name, modelUUID, err)
	}
	return nil
}

// LastModelLogin returns when the specified user last connected to the
// specified model in UTC. The following errors can be returned:
// - [accesserrors.UserNameNotValid] when the username is not valid.
// - [accesserrors.UserNotFound] when the user cannot be found.
// - [modelerrors.NotFound] if no model by the given modelUUID exists.
// - [accesserrors.UserNeverAccessedModel] if there is no record of the user
// accessing the model.
func (st *UserState) LastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID) (time.Time, error) {
	db, err := st.DB()
	if err != nil {
		return time.Time{}, errors.Errorf("getting DB access: %w", err)
	}

	uuidStmt, err := st.getActiveUUIDStmt()
	if err != nil {
		return time.Time{}, errors.Capture(err)
	}

	getLastModelLoginTime := `
SELECT   time AS &dbModelLastLogin.time
FROM     model_last_login
WHERE    model_uuid = $dbModelLastLogin.model_uuid
AND      user_uuid = $dbModelLastLogin.user_uuid
ORDER BY time DESC LIMIT 1;
	`
	getLastModelLoginTimeStmt, err := st.Prepare(getLastModelLoginTime, dbModelLastLogin{})
	if err != nil {
		return time.Time{}, errors.Errorf("preparing select getLastModelLoginTime query: %w", err)
	}

	var lastConnection time.Time
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		userUUID, err := st.uuidForName(ctx, tx, uuidStmt, name)
		if err != nil {
			return errors.Capture(err)
		}

		mll := dbModelLastLogin{
			ModelUUID: modelUUID.String(),
			UserUUID:  userUUID.String(),
		}
		err = tx.Query(ctx, getLastModelLoginTimeStmt, mll).Get(&mll)
		if errors.Is(err, sql.ErrNoRows) {
			if exists, err := st.checkModelExists(ctx, tx, modelUUID); err != nil {
				return errors.Errorf("checking model exists: %w", err)
			} else if !exists {
				return modelerrors.NotFound
			}
			return accesserrors.UserNeverAccessedModel
		} else if err != nil {
			return errors.Errorf("running query getLastModelLoginTime: %w", err)
		}

		lastConnection = mll.Time
		if err != nil {
			return errors.Errorf("parsing time: %w", err)
		}

		return nil
	})
	if err != nil {
		return time.Time{}, errors.Capture(err)
	}
	return lastConnection.UTC(), nil
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
	name user.Name,
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
		return errors.Errorf("preparing insert defineUserAuthentication query: %w", err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, insertDefineUserAuthenticationStmt, sqlair.M{"name": name.Name(), "disabled": false}).Get(&outcome)
	if internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%q: %w", name, accesserrors.UserNotFound)
	} else if err != nil {
		return errors.Errorf("setting authentication for user %q: %w", name, err)
	}

	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Errorf("determining results of setting authentication for user %q: %w", name, err)
	}

	if affected == 0 {
		return errors.Errorf("%q: %w", name, accesserrors.UserAuthenticationDisabled)
	}
	return nil
}

// setPasswordHash sets the password hash and salt for the user with the
// supplied uuid. If the user does not exist an error that satisfies
// accesserrors.UserNotFound will be returned. If the user does not have their
// authentication enabled an error that satisfies
// accesserrors.UserAuthenticationDisabled will be returned.
func setPasswordHash(ctx context.Context, tx *sqlair.TX, name user.Name, passwordHash string, salt []byte) error {
	err := ensureUserAuthentication(ctx, tx, name)
	if err != nil {
		return errors.Errorf("setting password hash for user %q: %w", name, err)
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
		return errors.Errorf("preparing insert setPasswordHash query: %w", err)
	}

	err = tx.Query(ctx, insertSetPasswordHashStmt, sqlair.M{
		"name":          name.Name(),
		"password_hash": passwordHash,
		"password_salt": salt},
	).Run()
	if err != nil {
		return errors.Errorf("setting password hash for user %q: %w", name, err)
	}

	return nil
}

// setActivationKey sets the activation key for the user with the supplied uuid.
// If the user does not exist an error that satisfies accesserrors.UserNotFound will
// be returned. If the user does not have their authentication enabled an error
// that satisfies accesserrors.UserAuthenticationDisabled will be returned.
func setActivationKey(ctx context.Context, tx *sqlair.TX, name user.Name, activationKey []byte) error {
	err := ensureUserAuthentication(ctx, tx, name)
	if err != nil {
		return errors.Errorf("setting activation key for user %q: %w", name, err)
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
		return errors.Errorf("preparing insert setActivationKey query: %w", err)
	}

	err = tx.Query(ctx, insertSetActivationKeyStmt, sqlair.M{"name": name.Name(), "activation_key": activationKey}).Run()
	if err != nil {
		return errors.Errorf("setting activation key for user %q: %w", name, err)
	}

	return nil
}

func (st *UserState) uuidForName(
	ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, name user.Name,
) (user.UUID, error) {
	var dbUUID userUUID
	err := tx.Query(ctx, stmt, userName{Name: name.Name()}).Get(&dbUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.Errorf("active user %q: %w", name, accesserrors.UserNotFound)
		}
		return "", errors.Errorf("getting user %q: %w", name, err)
	}

	uuid := user.UUID(dbUUID.UUID)
	if err := uuid.Validate(); err != nil {
		return "", errors.Errorf("invalid UUID for %q: %w", name, accesserrors.UserNotFound)
	}
	return uuid, nil
}

// getActiveUUIDStmt returns a SQLair prepared statement
// for retrieving the UUID of an active user.
func (st *UserState) getActiveUUIDStmt() (*sqlair.Statement, error) {
	return st.Prepare(
		"SELECT &userUUID.uuid FROM user WHERE name = $userName.name AND IFNULL(removed, false) = false", userUUID{}, userName{})
}

// checkModelExists returns a bool indicating if the model with the given UUID
// exists in the db.
func (st *UserState) checkModelExists(ctx context.Context, tx *sqlair.TX, modelUUID coremodel.UUID) (bool, error) {
	stmt, err := st.Prepare(`
SELECT true AS &dbModelExists.exists
FROM model
WHERE model.uuid = $dbModelUUID.uuid`, dbModelUUID{}, dbModelExists{})
	if err != nil {
		return false, errors.Errorf("preparing select checkModelExists: %w", err)
	}
	var exists dbModelExists
	err = tx.Query(ctx, stmt, dbModelUUID{UUID: modelUUID.String()}).Get(&exists)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}
