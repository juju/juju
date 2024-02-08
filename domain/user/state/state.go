// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/internal/auth"
	databaseutils "github.com/juju/juju/internal/database"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// AddUser will add a new user to the database. If the user already exists,
// an error that satisfies usererrors.AlreadyExists will be returned. If the
// creator does not exist, an error that satisfies
// usererrors.UserCreatorUUIDNotFound will be returned.
func (st *State) AddUser(
	ctx context.Context,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(addUser(ctx, tx, uuid, name, displayName, creatorUUID))
	})
}

// AddUserWithPasswordHash will add a new user to the database with the
// provided password hash and salt. If the user already exists, an error that
// satisfies usererrors.AlreadyExists will be returned. If the creator does
// not exist that satisfies usererrors.UserCreatorUUIDNotFound will be returned.
func (st *State) AddUserWithPasswordHash(
	ctx context.Context,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	passwordHash string,
	salt []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(AddUserWithPassword(ctx, tx, uuid, name, displayName, creatorUUID, passwordHash, salt))
	})
}

// AddUserWithActivationKey will add a new user to the database with the
// provided activation key. If the user already exists an error that
// satisfies usererrors.AlreadyExists will be returned. if the users creator
// does not exist an error that satisfies usererrors.UserCreatorUUIDNotFound
// will be returned.
func (st *State) AddUserWithActivationKey(
	ctx context.Context,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	activationKey []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = addUser(ctx, tx, uuid, name, displayName, creatorUUID)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(setActivationKey(ctx, tx, name, activationKey))
	})
}

// GetUsers will retrieve a list of filtered users with authentication information
// (last login, disabled) from the database. If no users exist an empty slice
// will be returned.
func (st *State) GetUsers(ctx context.Context, creatorName string) ([]user.User, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Annotate(err, "getting DB access")
	}

	var usrs []user.User
	if creatorName == "" {
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			getAllUsersQuery := `
SELECT (
        user.uuid, user.name, user.display_name, user.created_by_uuid, user.created_at,
        user_authentication.last_login, user_authentication.disabled
       ) AS (&User.*),
       creator.name AS &User.created_by_name
FROM   user
       LEFT JOIN user_authentication 
       ON        user.uuid = user_authentication.user_uuid
       LEFT JOIN user AS creator 
       ON        user.created_by_uuid = creator.uuid
WHERE user.removed = false 
`

			selectGetAllUsersStmt, err := sqlair.Prepare(getAllUsersQuery, User{}, sqlair.M{})
			if err != nil {
				return errors.Annotate(err, "preparing select getAllUsers query")
			}

			var results []User
			err = tx.Query(ctx, selectGetAllUsersStmt).GetAll(&results)
			if err != nil {
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
	} else {
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			getFilteredUsersQuery := `
SELECT (
        user.uuid, user.name, user.display_name, 
        user.created_by_uuid, 
        user.created_at,
        user_authentication.last_login, user_authentication.disabled
       ) AS (&User.*)
FROM   user
       LEFT JOIN user_authentication 
       ON        user.uuid = user_authentication.user_uuid
WHERE removed = false 
AND   user.created_by_uuid = (
         SELECT uuid
         FROM   user
         WHERE  name = $M.creator_name
      )
`

			selectGetFilteredUsersStmt, err := sqlair.Prepare(getFilteredUsersQuery, User{}, sqlair.M{})
			if err != nil {
				return errors.Annotate(err, "preparing select getFilteredUsers query")
			}

			var results []User
			err = tx.Query(ctx, selectGetFilteredUsersStmt, sqlair.M{"creator_name": creatorName}).GetAll(&results)
			if err != nil {
				return errors.Annotate(err, "getting query results")
			}

			for _, result := range results {
				usrs = append(usrs, result.toCoreUser())
			}

			return nil
		})
		if err != nil {
			return nil, errors.Annotate(err, "getting filtered users")
		}
	}

	return usrs, nil
}

// GetUser will retrieve the user with authentication information (last login, disabled)
// specified by UUID from the database. If the user does not exist an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) GetUser(ctx context.Context, uuid user.UUID) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserQuery := `
SELECT (
        user.uuid, user.name, user.display_name, user.created_by_uuid, user.created_at,
        user_authentication.last_login, user_authentication.disabled
       ) AS (&User.*)
FROM   user
       LEFT JOIN user_authentication 
       ON        user.uuid = user_authentication.user_uuid
WHERE  user.uuid = $M.uuid
`

		selectGetUserStmt, err := sqlair.Prepare(getUserQuery, User{}, sqlair.M{})
		if err != nil {
			return errors.Annotate(err, "preparing select getUser query")
		}

		var result User
		err = tx.Query(ctx, selectGetUserStmt, sqlair.M{"uuid": uuid.String()}).Get(&result)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(usererrors.NotFound, "%q", uuid)
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

// GetUserByName will retrieve the user with authentication information (last login, disabled)
// specified by name from the database. If the user does not exist an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) GetUserByName(ctx context.Context, name string) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserByNameQuery := `
SELECT (
        user.uuid, user.name, user.display_name, user.created_by_uuid, user.created_at, 
        user_authentication.last_login, user_authentication.disabled
       ) AS (&User.*)
FROM   user
       LEFT JOIN user_authentication 
       ON        user.uuid = user_authentication.user_uuid
WHERE  user.name = $M.name 
AND    removed = false
`

		selectGetUserByNameStmt, err := sqlair.Prepare(getUserByNameQuery, User{}, sqlair.M{})
		if err != nil {
			return errors.Annotate(err, "preparing select getUserByName query")
		}

		var result User
		err = tx.Query(ctx, selectGetUserByNameStmt, sqlair.M{"name": name}).Get(&result)
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			return errors.Annotatef(usererrors.NotFound, "%q", name)
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

// GetUserByAuth will retrieve the user with checking authentication information
// specified by UUID and password from the database. If the user does not exist
// or the user does not authenticate an error that satisfies usererrors.Unauthorized
// will be returned.
func (st *State) GetUserByAuth(ctx context.Context, name string, password string) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserWithAuthQuery := `
SELECT (
        user.uuid, user.name, user.display_name, user.created_by_uuid, user.created_at, 
        user_password.password_hash, user_password.password_salt
       ) AS (&User.*)
FROM   user
       LEFT JOIN user_password 
       ON        user.uuid = user_password.user_uuid
WHERE  user.name = $M.name 
AND    removed = false
`

		selectGetUserByAuthStmt, err := sqlair.Prepare(getUserWithAuthQuery, User{}, sqlair.M{})
		if err != nil {
			return errors.Annotate(err, "preparing select getUserWithAuth query")
		}

		var result User
		err = tx.Query(ctx, selectGetUserByAuthStmt, sqlair.M{"name": name}).Get(&result)
		if err != nil {
			return errors.Annotatef(usererrors.Unauthorized, "%q", name)
		}

		passwordHash, err := auth.HashPassword(auth.NewPassword(password), result.PasswordSalt)
		if err != nil {
			return errors.Annotatef(err, "hashing password for user with name %q", name)
		} else if passwordHash != result.PasswordHash {
			return errors.Annotatef(usererrors.Unauthorized, "%q", name)
		}

		usr = result.toCoreUser()

		return nil
	})
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user with name %q", name)
	}

	return usr, nil
}

// RemoveUser marks the user as removed. This obviates the ability of a user
// to function, but keeps the user retaining provenance, i.e. auditing.
// RemoveUser will also remove any credentials and activation codes for the
// user. If no user exists for the given user name then an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) RemoveUser(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// remove password hash
		err = removePasswordHash(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "removing password hash for user %q", name)
		}

		// remove activation key
		err = removeActivationKey(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "removing activation key for user with uuid %q", name)
		}

		removeUserQuery := `
UPDATE user 
SET    removed = true 
WHERE  name = $M.name
`

		updateRemoveUserStmt, err := sqlair.Prepare(removeUserQuery, sqlair.M{})
		if err != nil {
			return errors.Annotate(err, "preparing update removeUser query")
		}

		query := tx.Query(ctx, updateRemoveUserStmt, sqlair.M{"name": name})
		err = query.Run()
		if err != nil {
			return errors.Annotatef(err, "removing user %q", name)
		}

		outcome := sqlair.Outcome{}
		if err := query.Get(&outcome); err != nil {
			return errors.Annotatef(err, "removing user %q", name)
		}

		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Annotatef(err, "determining results of removing user with uuid %q", name)
		} else if affected != 1 {
			return errors.Annotatef(usererrors.NotFound, "removing user %q", name)
		}

		return nil
	})
	if err != nil {
		return errors.Annotatef(err, "removing user %q", name)
	}

	return nil
}

// SetActivationKey removes any active passwords for the user and sets the
// activation key. If no user is found for the supplied user name an error
// is returned that satisfies usererrors.NotFound.
func (st *State) SetActivationKey(ctx context.Context, name string, activationKey []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "getting user %q", name)
		}
		if removed {
			return errors.Annotatef(usererrors.NotFound, "%q", name)
		}

		err = removePasswordHash(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "removing password hash for user %q", name)
		}

		return errors.Trace(setActivationKey(ctx, tx, name, activationKey))
	})
}

// SetPasswordHash removes any active activation keys and sets the user
// password hash and salt. If no user is found for the supplied user name an error
// is returned that satisfies usererrors.NotFound.
func (st *State) SetPasswordHash(ctx context.Context, name string, passwordHash string, salt []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "getting user %q", name)
		}
		if removed {
			return errors.Annotatef(err, "setting password hash for removed user %q", name)
		}

		err = removeActivationKey(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "removing activation key for user %q", name)
		}

		return errors.Trace(setPasswordHash(ctx, tx, name, passwordHash, salt))
	})
}

// EnableUserAuthentication will enable the user with the supplied user name. If the user does not
// exist an error that satisfies usererrors.NotFound will be returned.
func (st *State) EnableUserAuthentication(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	enableUserQuery := `
INSERT INTO user_authentication (user_uuid, disabled)  
VALUES      (
                (
                    SELECT uuid
                    FROM   user
                    WHERE  name = $M.name 
                    AND    removed = false
                ), 
                $M.disabled
             )
  ON CONFLICT(user_uuid) DO UPDATE SET disabled = excluded.disabled
WHERE        disabled = true
`

	insertEnableUserStmt, err := sqlair.Prepare(enableUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing update enableUserAuthentication query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "getting user %q", name)
		}
		if removed {
			return errors.Annotatef(usererrors.NotFound, "%q", name)
		}

		query := tx.Query(ctx, insertEnableUserStmt, sqlair.M{"name": name, "disabled": false})
		err = query.Run()
		if err != nil {
			return errors.Annotatef(err, "enabling user authentication %q", name)
		}

		return nil
	})
}

// DisableUserAuthentication will disable the user with the supplied user name. If the user does
// not exist an error that satisfies usererrors.NotFound will be returned.
func (st *State) DisableUserAuthentication(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	disableUserQuery := `
INSERT INTO user_authentication (user_uuid, disabled) 
VALUES      (
                (
                    SELECT uuid
                    FROM   user
                    WHERE  name = $M.name 
                    AND    removed = false
                ), 
                $M.disabled
             )
  ON CONFLICT(user_uuid) DO UPDATE SET disabled = excluded.disabled
WHERE        disabled = false
`

	insertDisableUserStmt, err := sqlair.Prepare(disableUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing update disableUserAuthentication query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "getting user %q", name)
		}
		if removed {
			return errors.Annotatef(usererrors.NotFound, "%q", name)
		}

		query := tx.Query(ctx, insertDisableUserStmt, sqlair.M{"name": name, "disabled": true})
		err = query.Run()
		if err != nil {
			return errors.Annotatef(err, "disabling user authentication %q", name)
		}

		return nil
	})
}

// AddUserWithPassword adds a new user to the database with the
// provided password hash and salt. If the user already exists an error that
// satisfies usererrors.AlreadyExists will be returned. if the creator does
// not exist that satisfies usererrors.UserCreatorUUIDNotFound will be returned.
func AddUserWithPassword(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	passwordHash string,
	salt []byte,
) error {
	err := addUser(ctx, tx, uuid, name, displayName, creatorUUID)
	if err != nil {
		return errors.Annotatef(err, "adding user with uuid %q", uuid)
	}

	return errors.Trace(setPasswordHash(ctx, tx, name, passwordHash, salt))
}

// UpdateLastLogin updates the last login time for the user with the supplied
// uuid. If the user does not exist an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) UpdateLastLogin(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	updateLastLoginQuery := `
UPDATE user_authentication
SET    last_login = $M.last_login
WHERE user_uuid = (
          SELECT uuid
          FROM   user
          WHERE  name = $M.name 
          AND    removed = false
)
`

	updateLastLoginStmt, err := sqlair.Prepare(updateLastLoginQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing update updateLastLogin query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, name)
		if err != nil {
			return errors.Annotatef(err, "getting user %q", name)
		}
		if removed {
			return errors.Annotatef(usererrors.NotFound, "%q", name)
		}

		query := tx.Query(ctx, updateLastLoginStmt, sqlair.M{"name": name, "last_login": time.Now().UTC()})
		err = query.Run()
		if err != nil {
			return errors.Annotatef(err, "updating last login for user %q", name)
		}

		return nil
	})
}

// addUser adds a new user to the database. If the user already exists an error
// that satisfies usererrors.AlreadyExists will be returned. If the creator does
// not exist an error that satisfies usererrors.UserCreatorUUIDNotFound will be
// returned.
func addUser(ctx context.Context, tx *sqlair.TX, uuid user.UUID, name string, displayName string, creatorUuid user.UUID) error {
	addUserQuery := `
INSERT INTO user (uuid, name, display_name, created_by_uuid, created_at) 
VALUES      ($M.uuid, $M.name, $M.display_name, $M.created_by_uuid, $M.created_at)
`

	insertAddUserStmt, err := sqlair.Prepare(addUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert addUser query")
	}

	err = tx.Query(ctx, insertAddUserStmt, sqlair.M{
		"uuid":            uuid.String(),
		"name":            name,
		"display_name":    displayName,
		"created_by_uuid": creatorUuid.String(),
		"created_at":      time.Now(),
	}).Run()
	if databaseutils.IsErrConstraintUnique(err) {
		return errors.Annotatef(usererrors.AlreadyExists, "adding user %q", name)
	} else if databaseutils.IsErrConstraintForeignKey(err) {
		return errors.Annotatef(usererrors.UserCreatorUUIDNotFound, "adding user %q", name)
	} else if err != nil {
		return errors.Annotatef(err, "adding user %q", name)
	}

	return nil
}

// ensureUserAuthentication ensures that the user for uuid has their
// authentication table record and that their authentication is currently not
// disabled. If a user does not exist for the supplied user name then an error is
// returned that satisfies [usererrors.NotFound]. Should the user currently have
// their authentication disable an error satisfying
// [usererrors.UserAuthenticationDisabled] is returned.
func ensureUserAuthentication(
	ctx context.Context,
	tx *sqlair.TX,
	name string,
) error {
	defineUserAuthenticationQuery := `
INSERT INTO user_authentication (user_uuid, disabled) 
VALUES      ( 
                (
                    SELECT uuid
                    FROM user
                    WHERE name = $M.name AND removed = false
                ), 
                $M.disabled
            )
  ON CONFLICT(user_uuid) DO UPDATE SET user_uuid = excluded.user_uuid
WHERE       disabled = false
`

	insertDefineUserAuthenticationStmt, err := sqlair.Prepare(defineUserAuthenticationQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert defineUserAuthentication query")
	}

	query := tx.Query(ctx, insertDefineUserAuthenticationStmt, sqlair.M{"name": name, "disabled": false})
	err = query.Run()
	if databaseutils.IsErrConstraintForeignKey(err) {
		return errors.Annotatef(usererrors.NotFound, "%q", name)
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
		return errors.Annotatef(usererrors.UserAuthenticationDisabled, "%q", name)
	}
	return nil
}

// setPasswordHash sets the password hash and salt for the user with the
// supplied uuid. If the user does not exist an error that satisfies
// usererrors.NotFound will be returned. If the user does not have their
// authentication enabled an error that satisfies
// usererrors.UserAuthenticationDisabled will be returned.
func setPasswordHash(ctx context.Context, tx *sqlair.TX, name string, passwordHash string, salt []byte) error {
	err := ensureUserAuthentication(ctx, tx, name)
	if err != nil {
		return errors.Annotatef(err, "setting password hash for user %q", name)
	}

	setPasswordHashQuery := `
INSERT INTO user_password (user_uuid, password_hash, password_salt) 
VALUES      (
                (
                    SELECT uuid
                    FROM   user
                    WHERE  name = $M.name 
                    AND    removed = false
                ), 
                $M.password_hash, 
                $M.password_salt
            )
ON CONFLICT(user_uuid) DO NOTHING
`

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

// removePasswordHash removes the password hash and salt for the user with the
// supplied uuid.
func removePasswordHash(ctx context.Context, tx *sqlair.TX, name string) error {
	removePasswordHashQuery := `
DELETE FROM user_password
WHERE       user_uuid = (
                SELECT uuid
                FROM user
                WHERE name = $M.name 
                AND   removed = false
            )
`

	deleteRemovePasswordHashStmt, err := sqlair.Prepare(removePasswordHashQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing delete removePasswordHash query")
	}

	err = tx.Query(ctx, deleteRemovePasswordHashStmt, sqlair.M{"name": name}).Run()
	if err != nil {
		return errors.Annotatef(err, "removing password hash for user %q", name)
	}

	return nil
}

// setActivationKey sets the activation key for the user with the supplied uuid.
// If the user does not exist an error that satisfies usererrors.NotFound will
// be returned. If the user does not have their authentication enabled an error
// that satisfies usererrors.UserAuthenticationDisabled will be returned.
func setActivationKey(ctx context.Context, tx *sqlair.TX, name string, activationKey []byte) error {
	err := ensureUserAuthentication(ctx, tx, name)
	if err != nil {
		return errors.Annotatef(err, "setting activation key for user %q", name)
	}

	setActivationKeyQuery := `
INSERT INTO user_activation_key (user_uuid, activation_key)
VALUES      (
                (
                    SELECT uuid
                    FROM   user
                    WHERE  name = $M.name 
                    AND    removed = false
                ), 
             $M.activation_key
)
  ON CONFLICT(user_uuid) DO UPDATE SET activation_key = excluded.activation_key
`

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

// removeActivationKey removes the activation key for the user with the
// supplied uuid.
func removeActivationKey(ctx context.Context, tx *sqlair.TX, name string) error {
	removeActivationKeyQuery := `
DELETE FROM user_activation_key
WHERE       user_uuid = (
                SELECT uuid
                FROM   user
                WHERE  name = $M.name 
                AND    removed = false
            )
`

	deleteRemoveActivationKeyStmt, err := sqlair.Prepare(removeActivationKeyQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing delete removeActivationKey query")
	}

	err = tx.Query(ctx, deleteRemoveActivationKeyStmt, sqlair.M{"name": name}).Run()
	if err != nil {
		return errors.Annotatef(err, "remove activation key for user %q", name)
	}

	return nil
}

// isRemoved returns the value of the removed field for the user with the
// supplied uuid. If the user does not exist an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) isRemoved(ctx context.Context, tx *sqlair.TX, name string) (bool, error) {
	isRemovedQuery := `
SELECT removed AS &M.removed
FROM   user
WHERE  name = $M.name
`

	selectIsRemovedStmt, err := sqlair.Prepare(isRemovedQuery, sqlair.M{})
	if err != nil {
		return false, errors.Annotate(err, "preparing select isRemoved query")
	}

	var resultMap = sqlair.M{}
	err = tx.Query(ctx, selectIsRemovedStmt, sqlair.M{"name": name}).Get(&resultMap)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return false, errors.Annotatef(usererrors.NotFound, "%q", name)
	} else if err != nil {
		return false, errors.Annotatef(err, "getting user %q", name)
	}

	if resultMap["removed"] != nil {
		if removed, ok := resultMap["removed"].(bool); ok {
			return removed, nil
		}
	}

	return true, nil
}
