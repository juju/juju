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

// AddUser will add a new user to the database. If the user already exists
// an error that satisfies usererrors.AlreadyExists will be returned. If the
// creator does not exist an error that satisfies
// usererrors.UserCreatorUUIDNotFound will be returned.
func (st *State) AddUser(ctx context.Context, uuid user.UUID, user user.User, creatorUUID user.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(addUser(ctx, tx, uuid, user, creatorUUID))
	})
}

// AddUserWithPasswordHash will add a new user to the database with the
// provided password hash and salt. If the user already exists an error that
// satisfies usererrors.AlreadyExists will be returned. if the creator does
// not exist that satisfies usererrors.UserCreatorUUIDNotFound will be returned.
func (st *State) AddUserWithPasswordHash(
	ctx context.Context,
	uuid user.UUID,
	user user.User,
	creatorUUID user.UUID,
	passwordHash string,
	salt []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(AddUserWithPassword(ctx, tx, uuid, user, creatorUUID, passwordHash, salt))
	})
}

// AddUserWithActivationKey will add a new user to the database with the
// provided activation key. If the user already exists an error that
// satisfies usererrors.AlreadyExists will be returned. if the users creator
// does not exist an error that satisfies usererrors.UserCreatorNotFound
// will be returned.
func (st *State) AddUserWithActivationKey(
	ctx context.Context,
	uuid user.UUID,
	user user.User,
	creatorUUID user.UUID,
	activationKey []byte,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = addUser(ctx, tx, uuid, user, creatorUUID)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(setActivationKey(ctx, tx, uuid, activationKey))
	})
}

// GetUser will retrieve the user specified by UUID from the database.
// If the user does not exist an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) GetUser(ctx context.Context, uuid user.UUID) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserQuery := `
SELECT &User.*
FROM user
WHERE uuid = $M.uuid
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

// GetUserByName will retrieve the user specified by name from the database
// where the user is active and has not been removed. If the user does not
// exist or is removed an error that satisfies usererrors.NotFound will be
// returned.
func (st *State) GetUserByName(ctx context.Context, name string) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, errors.Annotate(err, "getting DB access")
	}

	var usr user.User
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		getUserByNameQuery := `
SELECT &User.*
FROM user
WHERE name = $M.name AND removed = false
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
			return errors.Annotatef(err, "getting user %q", name)
		}

		usr = result.toCoreUser()

		return nil
	})
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user %q", name)
	}

	return usr, nil
}

// RemoveUser marks the user as removed. This obviates the ability of a user
// to function, but keeps the user retaining provenance, i.e. auditing.
// RemoveUser will also remove any credentials and activation codes for the
// user. If no user exists for the given UUID then an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) RemoveUser(ctx context.Context, uuid user.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// remove password hash
		err = removePasswordHash(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "removing password hash for user with uuid %q", uuid)
		}

		// remove activation key
		err = removeActivationKey(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "removing activation key for user with uuid %q", uuid)
		}

		removeUserQuery := "UPDATE user SET removed = true WHERE uuid = $M.uuid"

		updateRemoveUserStmt, err := sqlair.Prepare(removeUserQuery, sqlair.M{})
		if err != nil {
			return errors.Annotate(err, "preparing update removeUser query")
		}

		query := tx.Query(ctx, updateRemoveUserStmt, sqlair.M{"uuid": uuid.String()})
		err = query.Run()
		if err != nil {
			return errors.Annotatef(err, "removing user with uuid %q", uuid)
		}

		outcome := sqlair.Outcome{}
		if err := query.Get(&outcome); err != nil {
			return errors.Annotatef(err, "removing user with uuid %q", uuid)
		}

		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Annotatef(err, "determining results of removing user with uuid %q", uuid)
		} else if affected != 1 {
			return errors.Annotatef(usererrors.NotFound, "removing user with uuid %q", uuid)
		}

		return nil
	})
	if err != nil {
		return errors.Annotatef(err, "removing user with uuid %q", uuid)
	}

	return nil
}

// SetActivationKey removes any active passwords for the user and sets the
// activation key. If no user is found for the supplied UUID an error
// is returned that satisfies usererrors.NotFound.
func (st *State) SetActivationKey(ctx context.Context, uuid user.UUID, activationKey []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "getting user with uuid %q", uuid)
		}
		if removed {
			return errors.Annotatef(usererrors.NotFound, "%q", uuid)
		}

		err = removePasswordHash(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "removing password hash for user with uuid %q", uuid)
		}

		return errors.Trace(setActivationKey(ctx, tx, uuid, activationKey))
	})
}

// SetPasswordHash removes any active activation keys and sets the user
// password hash and salt. If no user is found for the supplied UUID an error
// is returned that satisfies usererrors.NotFound.
func (st *State) SetPasswordHash(ctx context.Context, uuid user.UUID, passwordHash string, salt []byte) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "getting user with uuid %q", uuid)
		}
		if removed {
			return errors.Annotatef(err, "setting password hash for removed user with uuid %q", uuid)
		}

		err = removeActivationKey(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "removing activation key for user with uuid %q", uuid)
		}

		return errors.Trace(setPasswordHash(ctx, tx, uuid, passwordHash, salt))
	})
}

// EnableUserAuthentication will enable the user with the supplied UUID. If the user does not
// exist an error that satisfies usererrors.NotFound will be returned.
func (st *State) EnableUserAuthentication(ctx context.Context, uuid user.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	enableUserQuery := `
INSERT INTO user_authentication (user_uuid, disabled) VALUES ($M.uuid, $M.disabled)
ON CONFLICT(user_uuid) DO UPDATE SET disabled = excluded.disabled
WHERE disabled = true
`

	insertEnableUserStmt, err := sqlair.Prepare(enableUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing update enableUserAuthentication query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "getting user with uuid %q", uuid)
		}
		if removed {
			return errors.Annotatef(usererrors.NotFound, "%q", uuid)
		}

		query := tx.Query(ctx, insertEnableUserStmt, sqlair.M{"uuid": uuid.String(), "disabled": false})
		err = query.Run()
		if err != nil {
			return errors.Annotatef(err, "enabling user authentication with uuid %q", uuid)
		}

		return nil
	})
}

// DisableUserAuthentication will disable the user with the supplied UUID. If the user does
// not exist an error that satisfies usererrors.NotFound will be returned.
func (st *State) DisableUserAuthentication(ctx context.Context, uuid user.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting DB access")
	}

	disableUserQuery := `
INSERT INTO user_authentication (user_uuid, disabled) VALUES ($M.uuid, $M.disabled)
ON CONFLICT(user_uuid) DO UPDATE SET disabled = excluded.disabled
WHERE disabled = false
`

	insertDisableUserStmt, err := sqlair.Prepare(disableUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing update disableUserAuthentication query")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed, err := st.isRemoved(ctx, tx, uuid)
		if err != nil {
			return errors.Annotatef(err, "getting user with uuid %q", uuid)
		}
		if removed {
			return errors.Annotatef(usererrors.NotFound, "%q", uuid)
		}

		query := tx.Query(ctx, insertDisableUserStmt, sqlair.M{"uuid": uuid.String(), "disabled": true})
		err = query.Run()
		if err != nil {
			return errors.Annotatef(err, "disabling user authentication with uuid %q", uuid)
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
	usr user.User,
	creatorUUID user.UUID,
	passwordHash string,
	salt []byte,
) error {
	err := addUser(ctx, tx, uuid, usr, creatorUUID)
	if err != nil {
		return errors.Annotatef(err, "adding user with uuid %q", uuid)
	}

	return errors.Trace(setPasswordHash(ctx, tx, uuid, passwordHash, salt))
}

// addUser adds a new user to the database. If the user already exists an error
// that satisfies usererrors.AlreadyExists will be returned. If the creator does
// not exist an error that satisfies usererrors.UserCreatorUUIDNotFound will be
// returned.
func addUser(ctx context.Context, tx *sqlair.TX, uuid user.UUID, usr user.User, creatorUuid user.UUID) error {
	addUserQuery := `
INSERT INTO user (uuid, name, display_name, created_by_uuid, created_at) 
VALUES ($M.uuid, $M.name, $M.display_name, $M.created_by_uuid, $M.created_at)
`

	insertAddUserStmt, err := sqlair.Prepare(addUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert addUser query")
	}

	err = tx.Query(ctx, insertAddUserStmt, sqlair.M{
		"uuid":            uuid.String(),
		"name":            usr.Name,
		"display_name":    usr.DisplayName,
		"created_by_uuid": creatorUuid.String(),
		"created_at":      time.Now(),
	}).Run()
	if databaseutils.IsErrConstraintUnique(err) {
		return errors.Annotatef(usererrors.AlreadyExists, "adding user %q", usr.Name)
	} else if databaseutils.IsErrConstraintForeignKey(err) {
		return errors.Annotatef(usererrors.UserCreatorUUIDNotFound, "adding user %q", usr.Name)
	} else if err != nil {
		return errors.Annotatef(err, "adding user %q", usr.Name)
	}

	return nil
}

// ensureUserAuthentication ensures that the user for uuid has their
// authentication table record and that their authentication is currently not
// disabled. If a user does not exist for the supplied uuid then an error is
// returned that satisfies [usererrors.NotFound]. Should the user currently have
// their authentication disable an error satisfying
// [usererrors.UserAuthenticationDisabled] is returned.
func ensureUserAuthentication(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
) error {
	defineUserAuthenticationQuery := `
INSERT INTO user_authentication (user_uuid, disabled) VALUES ($M.uuid, $M.disabled)
ON CONFLICT(user_uuid) DO UPDATE SET user_uuid = excluded.user_uuid
WHERE disabled = false
`

	insertDefineUserAuthenticationStmt, err := sqlair.Prepare(defineUserAuthenticationQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert defineUserAuthentication query")
	}

	query := tx.Query(ctx, insertDefineUserAuthenticationStmt, sqlair.M{"uuid": uuid, "disabled": false})
	err = query.Run()
	if databaseutils.IsErrConstraintForeignKey(err) {
		return errors.Annotatef(usererrors.NotFound, "%q", uuid)
	} else if err != nil {
		return errors.Annotatef(err, "setting authentication for user with uuid %q", uuid)
	}

	outcome := sqlair.Outcome{}
	if err := query.Get(&outcome); err != nil {
		return errors.Annotatef(err, "setting authentication for user with uuid %q", uuid)
	}

	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Annotatef(err, "determining results of setting authentication for user with uuid %q", uuid)
	}

	if affected == 0 {
		return errors.Annotatef(usererrors.UserAuthenticationDisabled, "%q", uuid)
	}
	return nil
}

// setPasswordHash sets the password hash and salt for the user with the
// supplied uuid. If the user does not exist an error that satisfies
// usererrors.NotFound will be returned. If the user does not have their
// authentication enabled an error that satisfies
// usererrors.UserAuthenticationDisabled will be returned.
func setPasswordHash(ctx context.Context, tx *sqlair.TX, uuid user.UUID, passwordHash string, salt []byte) error {
	err := ensureUserAuthentication(ctx, tx, uuid)
	if err != nil {
		return errors.Annotatef(err, "setting password hash for user with uuid %q", uuid)
	}

	setPasswordHashQuery := `
INSERT INTO user_password (user_uuid, password_hash, password_salt) 
VALUES ($M.uuid, $M.password_hash, $M.password_salt) 
ON CONFLICT(user_uuid) DO NOTHING
`

	insertSetPasswordHashStmt, err := sqlair.Prepare(setPasswordHashQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert setPasswordHash query")
	}

	err = tx.Query(ctx, insertSetPasswordHashStmt, sqlair.M{
		"uuid":          uuid.String(),
		"password_hash": passwordHash,
		"password_salt": salt},
	).Run()
	if err != nil {
		return errors.Annotatef(err, "setting password hash for user with uuid %q", uuid)
	}

	return nil
}

// removePasswordHash removes the password hash and salt for the user with the
// supplied uuid.
func removePasswordHash(ctx context.Context, tx *sqlair.TX, uuid user.UUID) error {
	removePasswordHashQuery := `
DELETE FROM user_password
WHERE user_uuid = $M.uuid
`

	deleteRemovePasswordHashStmt, err := sqlair.Prepare(removePasswordHashQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing delete removePasswordHash query")
	}

	err = tx.Query(ctx, deleteRemovePasswordHashStmt, sqlair.M{"uuid": uuid.String()}).Run()
	if err != nil {
		return errors.Annotatef(err, "removing password hash for user with uuid %q", uuid)
	}

	return nil
}

// setActivationKey sets the activation key for the user with the supplied uuid.
// If the user does not exist an error that satisfies usererrors.NotFound will
// be returned. If the user does not have their authentication enabled an error
// that satisfies usererrors.UserAuthenticationDisabled will be returned.
func setActivationKey(ctx context.Context, tx *sqlair.TX, uuid user.UUID, activationKey []byte) error {
	err := ensureUserAuthentication(ctx, tx, uuid)
	if err != nil {
		return errors.Annotatef(err, "setting activation key for user with uuid %q", uuid)
	}

	setActivationKeyQuery := `
INSERT INTO user_activation_key (user_uuid, activation_key)
VALUES ($M.uuid, $M.activation_key)
ON CONFLICT(user_uuid) DO UPDATE SET activation_key = excluded.activation_key
`

	insertSetActivationKeyStmt, err := sqlair.Prepare(setActivationKeyQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing insert setActivationKey query")
	}

	err = tx.Query(ctx, insertSetActivationKeyStmt, sqlair.M{"uuid": uuid, "activation_key": activationKey}).Run()
	if err != nil {
		return errors.Annotatef(err, "setting activation key for user with uuid %q", uuid)
	}

	return nil
}

// removeActivationKey removes the activation key for the user with the
// supplied uuid.
func removeActivationKey(ctx context.Context, tx *sqlair.TX, uuid user.UUID) error {
	removeActivationKeyQuery := `
DELETE FROM user_activation_key
WHERE user_uuid = $M.uuid
`

	deleteRemoveActivationKeyStmt, err := sqlair.Prepare(removeActivationKeyQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing delete removeActivationKey query")
	}

	err = tx.Query(ctx, deleteRemoveActivationKeyStmt, sqlair.M{"uuid": uuid.String()}).Run()
	if err != nil {
		return errors.Annotatef(err, "remove activation key for user with uuid %q", uuid)
	}

	return nil
}

// isRemoved returns the value of the removed field for the user with the
// supplied uuid. If the user does not exist an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) isRemoved(ctx context.Context, tx *sqlair.TX, uuid user.UUID) (bool, error) {
	isRemovedQuery := `
SELECT removed AS &M.removed
FROM user 
WHERE uuid = $M.uuid
`

	selectIsRemovedStmt, err := sqlair.Prepare(isRemovedQuery, sqlair.M{})
	if err != nil {
		return false, errors.Annotate(err, "preparing select isRemoved query")
	}

	var resultMap = sqlair.M{}
	err = tx.Query(ctx, selectIsRemovedStmt, sqlair.M{"uuid": uuid.String()}).Get(&resultMap)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return false, errors.Annotatef(usererrors.NotFound, "%q", uuid)
	} else if err != nil {
		return false, errors.Annotatef(err, "getting user with uuid %q", uuid)
	}

	if resultMap["removed"] != nil {
		if removed, ok := resultMap["removed"].(bool); ok {
			return removed, nil
		}
	}

	return true, nil
}
