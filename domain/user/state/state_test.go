// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	schematesting "github.com/juju/juju/domain/schema/testing"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/database"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

// TestSingletonActiveUser asserts the idx_singleton_active_user unique index
// in the DDL. What we need in the DDL is the ability to have multiple users
// with the same username. However, only one username can exist in the table
// where the username has not been removed. We are free to have as many removed
// identical usernames as we want.
//
// This test will make 3 users called "bob" that have all been removed. This
// check asserts that we can have more than one removed bob.
// We will then add another user called "bob" that is not removed
// (an active user). This should not fail.
// We will then try and add a 5 user called "bob" that is also not removed and
// this will produce a unique index constraint error.
func (s *stateSuite) TestSingletonActiveUser(c *gc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "123", "bob", "Bob", true, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "124", "bob", "Bob", true, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "125", "bob", "Bob", true, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Insert the first non-removed (active) Bob user.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "126", "bob", "Bob", false, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Try and insert the second non-removed (active) Bob user. This should blow
	// up the constraint.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "127", "bob", "Bob", false, "123", time.Now())
		return err
	})
	c.Assert(database.IsErrConstraintUnique(err), jc.IsTrue)
}

// TestAddJujuSystemUser asserts that we can add a juju-system user to the
// database.
func (s *stateSuite) TestAddJujuSystemUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the user was added correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT uuid, name, display_name, removed, created_by_uuid, created_at 
FROM user 
WHERE uuid = ?
	`, jujuSystemUuid)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var uuid, name, displayName string
	var creatorUUID user.UUID
	var removed bool
	var createdAt time.Time
	err = row.Scan(&uuid, &name, &displayName, &removed, &creatorUUID, &createdAt)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, jujuSystemUser.Name)
	c.Check(removed, gc.Equals, false)
	c.Check(displayName, gc.Equals, jujuSystemUser.DisplayName)
	c.Check(creatorUUID, gc.Equals, jujuSystemCreatorUuid)
	c.Check(createdAt, gc.NotNil)
}

// TestAddAdminUser asserts that we can add an admin user owned by juju-system user to the database.
func (s *stateSuite) TestAddAdminUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	adminUserUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	adminuserCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}
	err = st.AddUser(context.Background(), adminUserUUID, adminUser, adminuserCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the user was added correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT uuid, name, display_name, removed, created_by_uuid, created_at 
FROM user 
WHERE uuid = ?
	`, adminUserUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var uuid, name, displayName string
	var creatorUUID user.UUID
	var removed bool
	var createdAt time.Time
	err = row.Scan(&uuid, &name, &displayName, &removed, &creatorUUID, &createdAt)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, adminUser.Name)
	c.Check(removed, gc.Equals, false)
	c.Check(displayName, gc.Equals, adminUser.DisplayName)
	c.Check(creatorUUID, gc.Equals, adminuserCreatorUuid)
	c.Check(createdAt, gc.NotNil)
}

// TestAddUserAlreadyExists asserts that we get an error when we try to add a
// user that already exists.
func (s *stateSuite) TestAddUserAlreadyExists(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}
	err = st.AddUser(context.Background(), userUUID, adminUser, userCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user again.
	err = st.AddUser(context.Background(), userUUID, adminUser, userCreatorUuid)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)
}

// TestAddUserWhichCreatorDoesNotExist asserts that we get an error when we try
// to add a user that has a creator that does not exist.
func (s *stateSuite) TestAddUserWhichCreatorDoesNotExist(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}

	// Try and add admin user with a creator that does not exist.
	err = st.AddUser(context.Background(), userUUID, adminUser, userCreatorUuid)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestGetUser asserts that we can get a user from the database.
func (s *stateSuite) TestGetUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}
	err = st.AddUser(context.Background(), userUUID, adminUser, userCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUser(context.Background(), userUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, adminUser.Name)
	c.Check(u.DisplayName, gc.Equals, adminUser.DisplayName)
	c.Check(u.CreatorUUID, gc.Equals, userCreatorUuid)
	c.Check(u.CreatedAt, gc.NotNil)
}

// TestGetUserNotFound asserts that we get an error when we try to get a user
// that does not exist.
func (s *stateSuite) TestGetUserNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Generate a random UUID.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUser(context.Background(), userUUID)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestGetUserByName asserts that we can get a user by name from the database.
func (s *stateSuite) TestGetUserByName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUUID := jujuSystemUUID
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUUID, jujuSystemUser, jujuSystemCreatorUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	adminCreatorUUID := jujuSystemUUID
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}
	err = st.AddUser(context.Background(), adminUUID, adminUser, adminCreatorUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(context.Background(), adminUser.Name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, adminUser.Name)
	c.Check(u.DisplayName, gc.Equals, adminUser.DisplayName)
	c.Check(u.CreatorUUID, gc.Equals, adminCreatorUUID)
	c.Check(u.CreatedAt, gc.NotNil)
}

// TestGetUserByNameNotFound asserts that we get an error when we try to get a
// user by name that does not exist.
func (s *stateSuite) TestGetUserByNameNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Get the user.
	_, err := st.GetUserByName(context.Background(), "admin")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestRemoveUser asserts that we can remove a user from the database.
func (s *stateSuite) TestRemoveUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}
	err = st.AddUser(context.Background(), userUUID, adminUser, userCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userToRemove := user.User{
		Name:        "userToRemove",
		DisplayName: "userToRemove",
	}
	err = st.AddUser(context.Background(), userToRemoveUUID, userToRemove, userUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), userToRemoveUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the user was removed correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT removed
FROM user
WHERE uuid = ?
	`, userToRemoveUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var removed bool
	err = row.Scan(&removed)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(removed, gc.Equals, true)
}

func (s *stateSuite) TestSetPasswordHash(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}
	err = st.AddUser(context.Background(), userUUID, adminUser, userCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Set password hash.
	err = st.SetPasswordHash(context.Background(), userUUID, "passwordHash", salt)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	rowAuth := db.QueryRow(`
SELECT disabled
FROM user_authentication
WHERE user_uuid = ?
	`, userUUID)
	c.Assert(rowAuth.Err(), jc.ErrorIsNil)

	var disabled bool
	err = rowAuth.Scan(&disabled)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(disabled, gc.Equals, false)

	row := db.QueryRow(`
SELECT password_hash
FROM user_password
WHERE user_uuid = ?
	`, userUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var passwordHash string
	err = row.Scan(&passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(passwordHash, gc.Equals, "passwordHash")

	row = db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, userUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestAddUserWithPasswordHash(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(context.Background(), userUUID, adminUser, userCreatorUuid, "passwordHash", salt)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT password_hash
FROM user_password
WHERE user_uuid = ?
	`, userUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var passwordHash string
	err = row.Scan(&passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(passwordHash, gc.Equals, "passwordHash")
}

// TestAddUserWithPasswordWhichCreatorDoesNotExist asserts that we get an error
// when we try to add a user with a password that has a creator that does not
// exist.
func (s *stateSuite) TestAddUserWithPasswordWhichCreatorDoesNotExist(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user with a creator that does not exist.
	err = st.AddUserWithPasswordHash(context.Background(), userUUID, adminUser, userCreatorUuid, "passwordHash", salt)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

func (s *stateSuite) TestAddUserWithActivationKey(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user with activation key.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}
	err = st.AddUserWithActivationKey(context.Background(), userUUID, adminUser, userCreatorUuid, "activationKey")
	c.Assert(err, jc.ErrorIsNil)

	// Check that the activation key was set correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, userUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(activationKey, gc.Equals, "activationKey")
}

// TestAddUserWithActivationKeyWhichCreatorDoesNotExist asserts that we get an
// error when we try to add a user with an activation key that has a creator
// that does not exist.
func (s *stateSuite) TestAddUserWithActivationKeyWhichCreatorDoesNotExist(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}

	// Try and add admin user with an activation key with a creator that does not exist.
	err = st.AddUserWithActivationKey(context.Background(), userUUID, adminUser, userCreatorUuid, "activationKey")
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

func (s *stateSuite) TestSetActivationKey(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add juju-system user.
	jujuSystemUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	jujuSystemCreatorUuid := jujuSystemUuid
	jujuSystemUser := user.User{
		Name:        "juju-system",
		DisplayName: "juju-system",
	}
	err = st.AddUser(context.Background(), jujuSystemUuid, jujuSystemUser, jujuSystemCreatorUuid)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin user.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userCreatorUuid := jujuSystemUuid
	adminUser := user.User{
		Name:        "admin",
		DisplayName: "admin",
	}

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(context.Background(), userUUID, adminUser, userCreatorUuid, "passwordHash", salt)
	c.Assert(err, jc.ErrorIsNil)

	// Set activation key.
	err = st.SetActivationKey(context.Background(), userUUID, "activationKey")
	c.Assert(err, jc.ErrorIsNil)

	// Check that the activation key was set correctly, and the password hash was removed.
	db := s.DB()

	row := db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, userUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(activationKey, gc.Equals, "activationKey")

	row = db.QueryRow(`
SELECT password_hash, password_salt
FROM user_password
WHERE user_uuid = ?
	`, userUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var passwordHash, passwordSalt string
	err = row.Scan(&passwordHash, &passwordSalt)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}
