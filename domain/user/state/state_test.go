// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"crypto/rand"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
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

func generateActivationKey() ([]byte, error) {
	var activationKey [32]byte
	if _, err := rand.Read(activationKey[:]); err != nil {
		return nil, errors.Annotate(err, "generating activation key")
	}
	return activationKey[:], nil
}

// AddUserWithPassword asserts that we can add a user with no
// password authorization.
func (s *stateSuite) TestBootstrapAddUserWithPassword(c *gc.C) {
	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with no password authorization.
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err = AddUserWithPassword(
			context.Background(), tx, adminUUID,
			"admin", "admin",
			adminUUID, "passwordHash", salt,
		)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the user was added correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT uuid, name, display_name, removed, created_by_uuid, created_at
FROM user
WHERE uuid = ?
	`, adminUUID)

	c.Assert(row.Err(), jc.ErrorIsNil)

	var uuid, name, displayName string
	var creatorUUID user.UUID
	var removed bool
	var createdAt time.Time
	err = row.Scan(&uuid, &name, &displayName, &removed, &creatorUUID, &createdAt)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(name, gc.Equals, "admin")
	c.Check(removed, gc.Equals, false)
	c.Check(displayName, gc.Equals, "admin")
	c.Check(creatorUUID, gc.Equals, adminUUID)
	c.Check(createdAt, gc.NotNil)
}

// TestAddUserAlreadyExists asserts that we get an error when we try to add a
// user that already exists.
func (s *stateSuite) TestAddUserAlreadyExists(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUser(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user again.
	adminCloneUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddUser(
		context.Background(), adminCloneUUID,
		"admin", "admin",
		adminCloneUUID,
	)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)
}

// TestAddUserCreatorNotFound asserts that we get an error when we try
// to add a user that has a creator that does not exist.
func (s *stateSuite) TestAddUserCreatorNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user with a creator that does not exist.
	nonExistingUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddUser(
		context.Background(), adminUUID,
		"admin", "admin",
		nonExistingUUID,
	)
	c.Assert(err, jc.ErrorIs, usererrors.CreatorUUIDNotFound)
}

// TestGetUser asserts that we can get a user from the database.
func (s *stateSuite) TestGetUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUser(context.Background(), adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, "admin")
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatedAt, gc.NotNil)
}

// TestGetRemovedUser asserts that we can get a removed user from the database.
func (s *stateSuite) TestGetRemovedUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUser(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(), userToRemoveUUID,
		"userToRemove", "userToRemove",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), "userToRemove")
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUser(context.Background(), userToRemoveUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, "userToRemove")
	c.Check(u.DisplayName, gc.Equals, "userToRemove")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
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

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, "admin")
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.LastLogin, gc.NotNil)
	c.Check(u.Disabled, gc.Equals, false)
}

// TestGetRemovedUserByName asserts that we can get only non-removed user by name.
func (s *stateSuite) TestGetRemovedUserByName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUser(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUser(
		context.Background(), userToRemoveUUID,
		"userToRemove", "userToRemove",
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), "userToRemove")
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByName(context.Background(), "userToRemove")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestGetUserByNameMultipleUsers asserts that we get a non-removed user when we try to
// get a user by name that has multiple users with the same name.
func (s *stateSuite) TestGetUserByNameMultipleUsers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUser(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove admin user.
	err = st.RemoveUser(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	// Add admin2 user.
	admin2UUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(),
		admin2UUID,
		"admin", "admin2",
		admin2UUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, "admin")
	c.Check(u.DisplayName, gc.Equals, "admin2")
	c.Check(u.CreatorUUID, gc.Equals, admin2UUID)
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

// TestGetUserWithAuthInfoByName asserts that we can get a user with auth info
// by name from the database.
func (s *stateSuite) TestGetUserWithAuthInfoByName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, "admin")
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.LastLogin, gc.NotNil)
	c.Check(u.Disabled, gc.Equals, false)
}

// TestGetUserByAuth asserts that we can get a user by auth from the database.
func (s *stateSuite) TestGetUserByAuth(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(context.Background(), adminUUID, "admin", "admin", adminUUID, passwordHash, salt)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByAuth(context.Background(), "admin", auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, "admin")
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatedAt, gc.NotNil)
}

// TestGetUserByAuthUnauthorized asserts that we get an error when we try to
// get a user by auth with the wrong password.
func (s *stateSuite) TestGetUserByAuthUnauthorized(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, passwordHash, salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByAuth(context.Background(), "admin", auth.NewPassword("wrong"))
	c.Assert(err, jc.ErrorIs, usererrors.Unauthorized)
}

// TestGetUserByAutUnexcitingUser asserts that we get an error when we try to
// get a user by auth that does not exist.
func (s *stateSuite) TestGetUserByAuthNotExtantUnauthorized(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Get the user.
	_, err := st.GetUserByAuth(context.Background(), "admin", auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIs, usererrors.Unauthorized)
}

// TestRemoveUser asserts that we can remove a user from the database.
func (s *stateSuite) TestRemoveUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUser(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUser(
		context.Background(), userToRemoveUUID,
		"userToRemove", "userToRemove",
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), "userToRemove")
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

// TestGetAllUsersWihAuthInfo asserts that we can get all users with auth info from
// the database.
func (s *stateSuite) TestGetAllUsersWihAuthInfo(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin1 user with password hash.
	admin1UUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(), admin1UUID,
		"admin1", "admin1",
		admin1UUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin2 user with activation key.
	admin2UUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	admin2ActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithActivationKey(
		context.Background(), admin2UUID,
		"admin2", "admin2",
		admin2UUID, admin2ActivationKey,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Disable admin2 user.
	err = st.DisableUserAuthentication(context.Background(), "admin2")
	c.Assert(err, jc.ErrorIsNil)

	// Get all users with auth info.
	users, err := st.GetAllUsers(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(users, gc.HasLen, 2)

	c.Check(users[0].Name, gc.Equals, "admin1")
	c.Check(users[0].DisplayName, gc.Equals, "admin1")
	c.Check(users[0].CreatorUUID, gc.Equals, admin1UUID)
	c.Check(users[0].CreatorName, gc.Equals, "admin1")
	c.Check(users[0].CreatedAt, gc.NotNil)
	c.Check(users[0].LastLogin, gc.NotNil)
	c.Check(users[0].Disabled, gc.Equals, false)

	c.Check(users[1].Name, gc.Equals, "admin2")
	c.Check(users[1].DisplayName, gc.Equals, "admin2")
	c.Check(users[1].CreatorUUID, gc.Equals, admin2UUID)
	c.Check(users[1].CreatorName, gc.Equals, "admin2")
	c.Check(users[1].CreatedAt, gc.NotNil)
	c.Check(users[1].LastLogin, gc.NotNil)
	c.Check(users[1].Disabled, gc.Equals, true)
}

// TestUserWithAuthInfo asserts that we can get a user with auth info from the
// database.
func (s *stateSuite) TestUserWithAuthInfo(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := "newguy"

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(context.Background(), uuid, name, name, uuid, "passwordHash", salt)
	c.Assert(err, jc.ErrorIsNil)

	err = st.DisableUserAuthentication(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	u, err := st.GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, name)
	c.Check(u.CreatorUUID, gc.Equals, uuid)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.LastLogin, gc.NotNil)
	c.Check(u.Disabled, gc.Equals, true)
}

// TestSetPasswordHash asserts that we can set a password hash for a user.
func (s *stateSuite) TestSetPasswordHash(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	newActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddUserWithActivationKey(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, newActivationKey,
	)
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Set password hash.
	err = st.SetPasswordHash(context.Background(), "admin", "passwordHash", salt)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	rowAuth := db.QueryRow(`
SELECT disabled
FROM user_authentication
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(rowAuth.Err(), jc.ErrorIsNil)

	var disabled bool
	err = rowAuth.Scan(&disabled)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(disabled, gc.Equals, false)

	row := db.QueryRow(`
SELECT password_hash
FROM user_password
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var passwordHash string
	err = row.Scan(&passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(passwordHash, gc.Equals, "passwordHash")

	row = db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

// TestAddUserWithPasswordHash asserts that we can add a user with a password
// hash.
func (s *stateSuite) TestAddUserWithPasswordHash(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	row := db.QueryRow(`SELECT password_hash FROM user_password WHERE user_uuid = ?`, adminUUID)
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
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	nonExistedCreatorUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user with a creator that does not exist.
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		nonExistedCreatorUuid, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIs, usererrors.CreatorUUIDNotFound)
}

// TestAddUserWithActivationKey asserts that we can add a user with an
// activation key.
func (s *stateSuite) TestAddUserWithActivationKey(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	adminActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddUserWithActivationKey(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, adminActivationKey,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the activation key was set correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(activationKey, gc.Equals, string(adminActivationKey))
}

// TestAddUserWithActivationKeyWhichCreatorDoesNotExist asserts that we get an
// error when we try to add a user with an activation key that has a creator
// that does not exist.
func (s *stateSuite) TestAddUserWithActivationKeyWhichCreatorDoesNotExist(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	nonExistedCreatorUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user with an activation key with a creator that does not exist.
	newActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddUserWithActivationKey(
		context.Background(), adminUUID,
		"admin", "admin",
		nonExistedCreatorUuid, newActivationKey,
	)
	c.Assert(err, jc.ErrorIs, usererrors.CreatorUUIDNotFound)
}

// TestSetActivationKey asserts that we can set an activation key for a user.
func (s *stateSuite) TestSetActivationKey(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Set activation key.
	adminActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetActivationKey(context.Background(), "admin", adminActivationKey)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the activation key was set correctly, and the password hash was removed.
	db := s.DB()

	row := db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(activationKey, gc.Equals, string(adminActivationKey))

	row = db.QueryRow(`
SELECT password_hash, password_salt
FROM user_password
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var passwordHash, passwordSalt string
	err = row.Scan(&passwordHash, &passwordSalt)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

// TestDisableUserAuthentication asserts that we can disable a user.
func (s *stateSuite) TestDisableUserAuthentication(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Disable user.
	err = st.DisableUserAuthentication(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	// Check that the user was disabled correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT disabled
FROM user_authentication
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var disabled bool
	err = row.Scan(&disabled)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(disabled, gc.Equals, true)
}

// TestEnableUserAuthentication asserts that we can enable a user.
func (s *stateSuite) TestEnableUserAuthentication(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Disable user.
	err = st.DisableUserAuthentication(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	// Enable user.
	err = st.EnableUserAuthentication(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	// Check that the user was enabled correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT disabled
FROM user_authentication
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var disabled bool
	err = row.Scan(&disabled)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(disabled, gc.Equals, false)
}

// TestUpdateLastLogin asserts that we can update the last login time for a
// user.
func (s *stateSuite) TestUpdateLastLogin(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		"admin", "admin",
		adminUUID, "passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Update last login.
	err = st.UpdateLastLogin(context.Background(), "admin")
	c.Assert(err, jc.ErrorIsNil)

	// Check that the last login was updated correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT last_login
FROM user_authentication
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var lastLogin time.Time
	err = row.Scan(&lastLogin)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(lastLogin, gc.NotNil)
}
