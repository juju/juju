// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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
// users creator is set and does not exist then an error that satisfies
// usererrors.UserCreatorNotFound will be returned.
func (st *State) AddUser(ctx context.Context, uuid user.UUID, user user.User, creatorUUID user.UUID) error {
	err := uuid.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate user uuid %q: %w", uuid, err)
	}
	err = creatorUUID.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate creator uuid %q: %w", creatorUUID, err)
	}

	db, err := st.DB()
	if err != nil {
		return fmt.Errorf("cannot get DB access: %w", err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return addUser(ctx, tx, uuid, user, creatorUUID)
	})
}

// AddUserWithPasswordHash will add a new user to the database with the
// provided password hash and salt. If the user already exists an error that
// satisfies usererrors.AlreadyExists will be returned. If the users creator
// does not exist or has been previously removed an error that satisfies
// usererrors.UserCreatorNotFound will be returned.
func (st *State) AddUserWithPasswordHash(
	ctx context.Context,
	uuid user.UUID,
	user user.User,
	creatorUUID user.UUID,
	passwordHash string,
	salt []byte,
) error {
	err := uuid.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate user uuid %q: %w", uuid, err)
	}
	err = creatorUUID.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate creator uuid %q: %w", creatorUUID, err)
	}

	db, err := st.DB()
	if err != nil {
		return fmt.Errorf("cannot get DB access: %w", err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err = addUser(ctx, tx, uuid, user, creatorUUID)
		if err != nil {
			return errors.Trace(err)
		}

		return setPasswordHash(ctx, tx, uuid, passwordHash, salt)
	})
}

// AddUserWithActivationKey will add a new user to the database with the
// provided activation key. If the user already exists an error that
// satisfies usererrors.AlreadyExists will be returned. if the users creator
// does not exist or has been previously removed an error that satisfies
// usererrors.UserCreatorNotFound will be returned.
func (st *State) AddUserWithActivationKey(
	ctx context.Context,
	uuid user.UUID,
	user user.User,
	creatorUUID user.UUID,
	activationKey string,
) error {
	err := uuid.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate user uuid %q: %w", uuid, err)
	}
	err = creatorUUID.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate creator uuid %q: %w", creatorUUID, err)
	}

	db, err := st.DB()
	if err != nil {
		return fmt.Errorf("cannot get DB access: %w", err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err = addUser(ctx, tx, uuid, user, creatorUUID)
		if err != nil {
			return errors.Trace(err)
		}
		return setActivationKey(ctx, tx, uuid, activationKey)
	})
	if err != nil {
		return fmt.Errorf("cannot add user %q with activation key: %w", user.Name, err)
	}

	return nil
}

// GetUser will retrieve the user specified by name from the database where
// the user is active and has not been removed. If the user does not exist
// or is deleted an error that satisfies usererrors.NotFound will be
// returned.
func (st *State) GetUser(ctx context.Context, uuid user.UUID) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, fmt.Errorf("cannot get DB access: %w", err)
	}

	var usr user.User
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		usr, err = getUser(ctx, tx, uuid)
		return errors.Trace(err)
	})
	if err != nil {
		return user.User{}, fmt.Errorf("cannot get user %q: %w", uuid, err)
	}

	return usr, nil
}

// GetUserByName will retrieve the user specified by name from the database
// where the user is active and has not been removed. If the user does not
// exist or is deleted an error that satisfies usererrors.NotFound will be
// returned.
func (st *State) GetUserByName(ctx context.Context, name string) (user.User, error) {
	db, err := st.DB()
	if err != nil {
		return user.User{}, fmt.Errorf("cannot get DB access: %w", err)
	}

	var usr user.User
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		usr, err = getUserByName(ctx, tx, name)
		return errors.Trace(err)
	})
	if err != nil {
		return user.User{}, fmt.Errorf("cannot get user %q: %w", name, err)
	}

	return usr, nil
}

// RemoveUser marks the user as removed. This obviates the ability of a user
// to function, but keeps the user retaining provenance, i.e. auditing.
// RemoveUser will also remove any credentials and activation codes for the
// user. If no user exists for the given name then an error that satisfies
// usererrors.NotFound will be returned.
func (st *State) RemoveUser(ctx context.Context, uuid user.UUID) error {
	err := uuid.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate user uuid %q: %w", uuid, err)
	}

	db, err := st.DB()
	if err != nil {
		return fmt.Errorf("cannot get DB access: %w", err)
	}

	removeUserQuery := "UPDATE user SET removed = true WHERE uuid = ?"

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, removeUserQuery, uuid.String())
		if num, err := result.RowsAffected(); err != nil {
			return usererrors.NotFound
		} else if num != 1 {
			return fmt.Errorf("expected to remove 1 user, but removed %d", num)
		}
		return errors.Trace(err)
	})
}

// SetActivationKey removes any active passwords for the user and sets the
// activation key. If no user is found for the supplied name an error
// is returned that satisfies usererrors.NotFound.
func (st *State) SetActivationKey(ctx context.Context, uuid user.UUID, activationKey string) error {
	err := uuid.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate user uuid %q: %w", uuid, err)
	}

	removed, err := st.isRemoved(ctx, uuid)
	if err != nil {
		return fmt.Errorf("cannot get user %q: %w", uuid, err)
	}
	if removed {
		return fmt.Errorf("cannot set activation key for removed user %q", uuid)
	}

	db, err := st.DB()
	if err != nil {
		return fmt.Errorf("cannot get DB access: %w", err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err = removePasswordHash(ctx, tx, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		return setActivationKey(ctx, tx, uuid, activationKey)
	})
}

// SetPasswordHash removes any active activation keys and sets the user
// password hash and salt. If no user is found for the supplied name an error
// is returned that satisfies usererrors.NotFound.
func (st *State) SetPasswordHash(ctx context.Context, uuid user.UUID, passwordHash string, salt []byte) error {
	err := uuid.Validate()
	if err != nil {
		return fmt.Errorf("cannot validate user uuid %q: %w", uuid, err)
	}

	removed, err := st.isRemoved(ctx, uuid)
	if err != nil {
		return fmt.Errorf("cannot get user uuid %q: %w", uuid, err)
	}
	if removed {
		return fmt.Errorf("cannot set password hash for removed user %q", uuid)
	}

	db, err := st.DB()
	if err != nil {
		return fmt.Errorf("cannot get DB access: %w", err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err = removeActivationKey(ctx, tx, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		return setPasswordHash(ctx, tx, uuid, passwordHash, salt)
	})
}

func addUser(ctx context.Context, tx *sql.Tx, uuid user.UUID, user user.User, creatorUuid user.UUID) error {
	addUserQuery := "INSERT INTO user (uuid, name, display_name, created_by_uuid, created_at) VALUES (?, ?, ?, ?, ?)"

	_, err := tx.ExecContext(ctx, addUserQuery,
		uuid.String(),
		user.Name,
		user.DisplayName,
		creatorUuid.String(),
		time.Now(),
	)
	if databaseutils.IsErrConstraintUnique(err) {
		return usererrors.AlreadyExists
	} else if databaseutils.IsErrConstraintForeignKey(err) {
		return usererrors.UserCreatorUUIDNotFound
	} else if err != nil {
		return fmt.Errorf("cannot insert user %q: %w", user.Name, err)
	}

	return nil
}

func setPasswordHash(ctx context.Context, tx *sql.Tx, uuid user.UUID, passwordHash string, salt []byte) error {
	defineUserAuthenticationQuery := `
INSERT INTO user_authentication (user_uuid, disabled) 
VALUES (?, ?) 
ON CONFLICT(user_uuid) DO UPDATE SET user_uuid = excluded.user_uuid, disabled = excluded.disabled
`

	_, err := tx.ExecContext(ctx, defineUserAuthenticationQuery, uuid, false)
	if err != nil {
		return fmt.Errorf("cannot set authentification for user uuid %q: %w", uuid, err)
	}

	setPasswordHashQuery := `
INSERT INTO user_password (user_uuid, password_hash, password_salt) 
VALUES (?, ?, ?) 
ON CONFLICT(user_uuid) DO UPDATE SET password_hash = excluded.password_hash, password_salt = excluded.password_salt
`

	_, err = tx.ExecContext(ctx, setPasswordHashQuery, uuid.String(), passwordHash, salt)
	if err != nil {
		return fmt.Errorf("cannot set password hash for user uuid %q: %w", uuid, err)
	}

	return nil
}

func removePasswordHash(ctx context.Context, tx *sql.Tx, uuid user.UUID) error {
	removePasswordHashQuery := `
DELETE FROM user_password
WHERE user_uuid = ?
`

	_, err := tx.ExecContext(ctx, removePasswordHashQuery, uuid.String())
	if err != nil {
		return fmt.Errorf("cannot remove password hash for user uuid %q: %w", uuid, err)
	}

	return nil
}

func removeActivationKey(ctx context.Context, tx *sql.Tx, uuid user.UUID) error {
	removeActivationKeyQuery := `
DELETE FROM user_activation_key
WHERE user_uuid = ?
`

	_, err := tx.ExecContext(ctx, removeActivationKeyQuery, uuid.String())
	if err != nil {
		return fmt.Errorf("cannot remove activation key for user %q: %w", uuid, err)
	}

	return nil
}

func getUser(ctx context.Context, tx *sql.Tx, uuid user.UUID) (user.User, error) {
	getUserQuery := `
SELECT name, display_name, created_by_uuid, created_at
FROM user
WHERE uuid = ?
`

	var usr user.User
	row := tx.QueryRowContext(ctx, getUserQuery, uuid)

	var name, displayName string
	var creatorUUID user.UUID
	var createdAt time.Time
	err := row.Scan(&name, &displayName, &creatorUUID, &createdAt)
	if err != nil {
		return user.User{}, usererrors.NotFound
	}
	usr = user.User{
		UUID:        uuid,
		Name:        name,
		DisplayName: displayName,
		CreatorUUID: creatorUUID,
		CreatedAt:   createdAt,
	}

	return usr, nil
}

func getUserByName(ctx context.Context, tx *sql.Tx, name string) (user.User, error) {
	getUserQuery := `
SELECT uuid, display_name, created_by_uuid, created_at
FROM user
WHERE name = ? AND removed = false
`

	var usr user.User
	row := tx.QueryRowContext(ctx, getUserQuery, name)

	var uuid user.UUID
	var displayName string
	var creatorUUID user.UUID
	var createdAt time.Time
	err := row.Scan(&uuid, &displayName, &creatorUUID, &createdAt)
	if err != nil {
		return user.User{}, usererrors.NotFound
	}
	usr = user.User{
		UUID:        uuid,
		Name:        name,
		DisplayName: displayName,
		CreatorUUID: creatorUUID,
		CreatedAt:   createdAt,
	}

	return usr, nil
}

func setActivationKey(ctx context.Context, tx *sql.Tx, uuid user.UUID, activationKey string) error {
	defineUserAuthenticationQuery := `
INSERT INTO user_authentication (user_uuid, disabled) 
VALUES (?, ?) 
ON CONFLICT(user_uuid) DO UPDATE SET user_uuid = excluded.user_uuid, disabled = excluded.disabled
`

	_, err := tx.ExecContext(ctx, defineUserAuthenticationQuery, uuid, false)
	if err != nil {
		return fmt.Errorf("cannot set authentification for user uuid %q: %w", uuid, err)
	}

	setActivationKeyQuery := `
INSERT INTO user_activation_key (user_uuid, activation_key)
VALUES (?, ?)
ON CONFLICT(user_uuid) DO UPDATE SET activation_key = excluded.activation_key
`

	_, err = tx.ExecContext(ctx, setActivationKeyQuery, uuid.String(), activationKey)
	if err != nil {
		return fmt.Errorf("cannot set activation key for user uuid %q: %w", uuid, err)
	}

	return nil
}

// isRemoved returns true if the user has been removed.
func (st *State) isRemoved(ctx context.Context, uuid user.UUID) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, fmt.Errorf("cannot get DB access: %w", err)
	}

	isRemovedQuery := "SELECT removed FROM user WHERE uuid = ?"

	var removed bool
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, isRemovedQuery, uuid.String())
		err = row.Scan(&removed)
		return errors.Trace(err)
	})

	return removed, errors.Trace(err)
}
