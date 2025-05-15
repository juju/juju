// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"crypto/rand"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"
	"golang.org/x/net/context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/keymanager"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type userStateSuite struct {
	schematesting.ControllerSuite

	controllerUUID string
}

var _ = tc.Suite(&userStateSuite{})

func (s *userStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.controllerUUID = s.SeedControllerUUID(c)
}

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
func (s *userStateSuite) TestSingletonActiveUser(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "123", "bob", "Bob", false, true, "123", time.Now())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "124", "bob", "Bob", false, true, "123", time.Now())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "125", "bob", "Bob", false, true, "123", time.Now())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Insert the first non-removed (active) Bob user.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "126", "bob", "Bob", false, false, "123", time.Now())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Try and insert the second non-removed (active) Bob user. This should blow
	// up the constraint.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "127", "bob", "Bob", false, false, "123", time.Now())
		return err
	})
	c.Assert(database.IsErrConstraintUnique(err), tc.IsTrue)
}

func generateActivationKey() ([]byte, error) {
	var activationKey [32]byte
	if _, err := rand.Read(activationKey[:]); err != nil {
		return nil, errors.Errorf("generating activation key: %w", err)
	}
	return activationKey[:], nil
}

// AddUserWithPassword asserts that we can add a user with no
// password authorization.
func (s *userStateSuite) TestBootstrapAddUserWithPassword(c *tc.C) {
	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Add user with no password authorization.
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err = AddUserWithPassword(
			c.Context(), tx, adminUUID,
			usertesting.GenNewName(c, "admin"), "admin",
			adminUUID, s.controllerLoginAccess(), "passwordHash", salt,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check that the user was added correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT uuid, name, display_name, removed, created_by_uuid, created_at
FROM user
WHERE uuid = ?
	`, adminUUID)

	c.Assert(row.Err(), tc.ErrorIsNil)

	var uuid, name, displayName string
	var creatorUUID user.UUID
	var removed bool
	var createdAt time.Time
	err = row.Scan(&uuid, &name, &displayName, &removed, &creatorUUID, &createdAt)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, "admin")
	c.Check(removed, tc.Equals, false)
	c.Check(displayName, tc.Equals, "admin")
	c.Check(creatorUUID, tc.Equals, adminUUID)
	c.Check(createdAt, tc.NotNil)
}

// TestAddUser asserts a new user is added, enabled, and has
// the provided permission.
func (s *userStateSuite) TestAddUser(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		c.Context(), adminUUID,
		name, "admin", false,
		adminUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	newUser, err := st.GetUser(c.Context(), adminUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(newUser.Name, tc.Equals, name)
	c.Check(newUser.UUID, tc.Equals, adminUUID)
	c.Check(newUser.Disabled, tc.IsFalse)
	c.Check(newUser.CreatorUUID, tc.Equals, adminUUID)
}

// TestAddUserAlreadyExists asserts that we get an error when we try to add a
// user that already exists.
func (s *userStateSuite) TestAddUserAlreadyExists(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		c.Context(), adminUUID,
		name, "admin", false,
		adminUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Try and add admin user again.
	adminCloneUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = st.AddUser(
		c.Context(), adminCloneUUID,
		name, "admin", false,
		adminCloneUUID,
	)
	c.Assert(err, tc.ErrorIs, usererrors.UserAlreadyExists)
}

// TestAddUserCreatorNotFound asserts that we get an error when we try
// to add a user that has a creator that does not exist.
func (s *userStateSuite) TestAddUserCreatorNotFound(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	// Try and add admin user with a creator that does not exist.
	nonExistingUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		c.Context(), adminUUID,
		name, "admin", false,
		nonExistingUUID,
	)
	c.Assert(err, tc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestAddUserWithPermission asserts a new user is added, enabled, and has
// the provided permission.
func (s *userStateSuite) TestAddUserWithPermission(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	loginAccess := s.controllerLoginAccess()
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		c.Context(), adminUUID,
		name, "admin", false,
		adminUUID, loginAccess,
	)
	c.Assert(err, tc.ErrorIsNil)

	newUser, err := st.GetUser(c.Context(), adminUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(newUser.Name, tc.Equals, name)
	c.Check(newUser.UUID, tc.Equals, adminUUID)
	c.Check(newUser.Disabled, tc.IsFalse)
	c.Check(newUser.CreatorUUID, tc.Equals, adminUUID)
	c.Check(newUser.CreatorName, tc.Equals, user.AdminUserName)

	pSt := NewPermissionState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	newUserAccess, err := pSt.ReadUserAccessForTarget(c.Context(), usertesting.GenNewName(c, "admin"), loginAccess.Target)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(newUserAccess.Access, tc.Equals, loginAccess.Access)
	c.Check(newUserAccess.UserName, tc.Equals, newUser.Name)
	c.Check(newUserAccess.Object, tc.Equals, loginAccess.Target)
}

// TestAddUserWithPermissionInvalid asserts that we can't add a user to the
// database.
func (s *userStateSuite) TestAddUserWithPermissionInvalid(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		permission.AccessSpec{
			Access: permission.ReadAccess,
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        "foo-bar",
			},
		},
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIs, usererrors.PermissionTargetInvalid)
}

// TestGetUser asserts that we can get a user from the database.
func (s *userStateSuite) TestGetUser(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUser(c.Context(), adminUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, name)
	c.Check(u.DisplayName, tc.Equals, "admin")
	c.Check(u.CreatorUUID, tc.Equals, adminUUID)
	c.Check(u.CreatorName, tc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, tc.NotNil)
}

// TestGetRemovedUser asserts that we can get a removed user from the database.
func (s *userStateSuite) TestGetRemovedUser(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		c.Context(), adminUUID,
		adminName, "admin", false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUserWithPasswordHash(
		c.Context(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(c.Context(), userToRemoveName)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUser(c.Context(), userToRemoveUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, userToRemoveName)
	c.Check(u.DisplayName, tc.Equals, "userToRemove")
	c.Check(u.CreatorUUID, tc.Equals, adminUUID)
	c.Check(u.CreatorName, tc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, tc.NotNil)
}

// TestGetUserNotFound asserts that we get an error when we try to get a user
// that does not exist.
func (s *userStateSuite) TestGetUserNotFound(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Generate a random UUID.
	userUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUser(c.Context(), userUUID)
	c.Assert(err, tc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUserByName asserts that we can get a user by name from the database.
func (s *userStateSuite) TestGetUserByName(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, name)
	c.Check(u.DisplayName, tc.Equals, "admin")
	c.Check(u.CreatorUUID, tc.Equals, adminUUID)
	c.Check(u.CreatorName, tc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, tc.NotNil)
	c.Check(u.LastLogin, tc.NotNil)
	c.Check(u.Disabled, tc.Equals, false)
}

// TestGetRemovedUserByName asserts that we can get only non-removed user by name.
func (s *userStateSuite) TestGetRemovedUserByName(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		c.Context(), adminUUID,
		adminName, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUserWithPermission(
		c.Context(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(c.Context(), userToRemoveName)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByName(c.Context(), userToRemoveName)
	c.Assert(err, tc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUserByNameMultipleUsers asserts that we get a non-removed user when we try to
// get a user by name that has multiple users with the same name.
func (s *userStateSuite) TestGetUserByNameMultipleUsers(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		c.Context(), adminUUID,
		name, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Remove admin user.
	err = st.RemoveUser(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// Add admin2 user.
	admin2UUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		c.Context(),
		admin2UUID,
		name, "admin2",
		admin2UUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, name)
	c.Check(u.DisplayName, tc.Equals, "admin2")
	c.Check(u.CreatorUUID, tc.Equals, admin2UUID)
	c.Check(u.CreatorName, tc.Equals, name)
	c.Check(u.CreatedAt, tc.NotNil)
}

// TestGetUserByNameNotFound asserts that we get an error when we try to get a
// user by name that does not exist.
func (s *userStateSuite) TestGetUserByNameNotFound(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Get the user.
	_, err := st.GetUserByName(c.Context(), usertesting.GenNewName(c, "admin"))
	c.Assert(err, tc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUserWithAuthInfoByName asserts that we can get a user with auth info
// by name from the database.
func (s *userStateSuite) TestGetUserWithAuthInfoByName(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, name)
	c.Check(u.DisplayName, tc.Equals, "admin")
	c.Check(u.CreatorUUID, tc.Equals, adminUUID)
	c.Check(u.CreatorName, tc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, tc.NotNil)
	c.Check(u.LastLogin, tc.NotNil)
	c.Check(u.Disabled, tc.Equals, false)
}

// TestGetUserByAuth asserts that we can get a user by auth from the database.
func (s *userStateSuite) TestGetUserByAuth(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(),
		adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		passwordHash, salt)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByAuth(c.Context(), name, auth.NewPassword("password"))
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, name)
	c.Check(u.DisplayName, tc.Equals, "admin")
	c.Check(u.CreatorUUID, tc.Equals, adminUUID)
	c.Check(u.CreatorName, tc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, tc.NotNil)
	c.Check(u.Disabled, tc.IsFalse)
}

// TestGetUserByAuthWithInvalidSalt asserts that we correctly send an
// unauthorized error if the user doesn't have a valid salt.
func (s *userStateSuite) TestGetUserByAuthWithInvalidSalt(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", []byte{},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByAuth(c.Context(), name, auth.NewPassword("passwordHash"))
	c.Assert(err, tc.ErrorIs, usererrors.UserUnauthorized)
}

// TestGetUserByAuthDisabled asserts that we can get a user by auth from the
// database and has the correct disabled flag.
func (s *userStateSuite) TestGetUserByAuthDisabled(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(),
		adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		passwordHash, salt)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DisableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByAuth(c.Context(), name, auth.NewPassword("password"))
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, name)
	c.Check(u.DisplayName, tc.Equals, "admin")
	c.Check(u.CreatorUUID, tc.Equals, adminUUID)
	c.Check(u.CreatorName, tc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, tc.NotNil)
	c.Check(u.Disabled, tc.IsTrue)
}

// TestGetUserByAuthUnauthorized asserts that we get an error when we try to
// get a user by auth with the wrong password.
func (s *userStateSuite) TestGetUserByAuthUnauthorized(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		passwordHash, salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByAuth(c.Context(), name, auth.NewPassword("wrong"))
	c.Assert(err, tc.ErrorIs, usererrors.UserUnauthorized)
}

// TestGetUserByAuthDoesNotExist asserts that we get an error when we try to
// get a user by auth that does not exist.
func (s *userStateSuite) TestGetUserByAuthDoesNotExist(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Get the user.
	_, err := st.GetUserByAuth(c.Context(), usertesting.GenNewName(c, "admin"), auth.NewPassword("password"))
	c.Assert(err, tc.ErrorIs, usererrors.UserNotFound)
}

// TestRemoveUser asserts that we can remove a user from the database.
func (s *userStateSuite) TestRemoveUser(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		c.Context(), adminUUID,
		adminName, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUserWithPermission(
		c.Context(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(c.Context(), userToRemoveName)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the user has been successfully removed.
	db := s.DB()

	// Check that the user password was removed
	row := db.QueryRow(`
SELECT user_uuid
FROM user_password
WHERE user_uuid = ?
	`, userToRemoveUUID)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), tc.ErrorIs, sql.ErrNoRows)

	// Check that the user activation key was removed
	row = db.QueryRow(`
SELECT user_uuid
FROM user_activation_key
WHERE user_uuid = ?
	`, userToRemoveUUID)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), tc.ErrorIs, sql.ErrNoRows)

	// Check that the user was marked as removed.
	row = db.QueryRow(`
SELECT removed
FROM user
WHERE uuid = ?
	`, userToRemoveUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var removed bool
	err = row.Scan(&removed)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.Equals, true)

}

// TestRemoveUserSSHKeys is here to test that when we remove a user from the
// Juju database we delete all ssh keys for the user.
func (s *userStateSuite) TestRemoveUserSSHKeys(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		c.Context(), adminUUID,
		adminName, "admin",
		false,
		adminUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUser(
		c.Context(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		false,
		adminUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	modelId := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")

	// Add a public key onto a model for the user.
	km := keymanagerstate.NewState(s.TxnRunnerFactory())
	err = km.AddPublicKeysForUser(c.Context(), modelId, userToRemoveUUID, []keymanager.PublicKey{
		{
			Comment:         "test",
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     "something",
			Key:             "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju2@example.com",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(c.Context(), userToRemoveName)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the user has been successfully removed.
	db := s.DB()

	row := db.QueryRow(`
SELECT model_uuid
FROM model_authorized_keys
`)
	c.Assert(row.Scan(nil), tc.ErrorIs, sql.ErrNoRows)

	row = db.QueryRow(`
SELECT id
FROM user_public_ssh_key
WHERE user_uuid = ?
`, userToRemoveUUID)
	c.Assert(row.Scan(nil), tc.ErrorIs, sql.ErrNoRows)

	// Check that the user password was removed
	row = db.QueryRow(`
SELECT user_uuid
FROM user_password
WHERE user_uuid = ?
	`, userToRemoveUUID)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), tc.ErrorIs, sql.ErrNoRows)

	// Check that the user activation key was removed
	row = db.QueryRow(`
SELECT user_uuid
FROM user_activation_key
WHERE user_uuid = ?
	`, userToRemoveUUID)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), tc.ErrorIs, sql.ErrNoRows)

	// Check that the user was marked as removed.
	row = db.QueryRow(`
SELECT removed
FROM user
WHERE uuid = ?
	`, userToRemoveUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var removed bool
	err = row.Scan(&removed)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removed, tc.Equals, true)

}

// TestGetAllUsersWihAuthInfo asserts that we can get all users with auth info from
// the database.
func (s *userStateSuite) TestGetAllUsersWihAuthInfo(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin1 user with password hash.
	admin1UUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	admin1Name := usertesting.GenNewName(c, "admin1")
	err = st.AddUserWithPasswordHash(
		c.Context(), admin1UUID,
		admin1Name, "admin1",
		admin1UUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add admin2 user with activation key.
	admin2UUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	admin2ActivationKey, err := generateActivationKey()
	c.Assert(err, tc.ErrorIsNil)

	admin2Name := usertesting.GenNewName(c, "admin2")
	err = st.AddUserWithActivationKey(
		c.Context(), admin2UUID,
		admin2Name, "admin2",
		admin2UUID,
		s.controllerLoginAccess(),
		admin2ActivationKey,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Disable admin2 user.
	err = st.DisableUserAuthentication(c.Context(), admin2Name)
	c.Assert(err, tc.ErrorIsNil)

	// Get all users with auth info, including disabled users.
	users, err := st.GetAllUsers(c.Context(), true)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(users, tc.HasLen, 2)

	c.Check(users[0].Name, tc.Equals, admin1Name)
	c.Check(users[0].DisplayName, tc.Equals, "admin1")
	c.Check(users[0].CreatorUUID, tc.Equals, admin1UUID)
	c.Check(users[0].CreatorName, tc.Equals, admin1Name)
	c.Check(users[0].CreatedAt, tc.NotNil)
	c.Check(users[0].LastLogin, tc.NotNil)
	c.Check(users[0].Disabled, tc.Equals, false)

	c.Check(users[1].Name, tc.Equals, admin2Name)
	c.Check(users[1].DisplayName, tc.Equals, "admin2")
	c.Check(users[1].CreatorUUID, tc.Equals, admin2UUID)
	c.Check(users[1].CreatorName, tc.Equals, admin2Name)
	c.Check(users[1].CreatedAt, tc.NotNil)
	c.Check(users[1].LastLogin, tc.NotNil)
	c.Check(users[1].Disabled, tc.Equals, true)

	// Get all users with auth info, excluding disabled users
	users, err = st.GetAllUsers(c.Context(), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(users, tc.HasLen, 1)

	c.Check(users[0].Name, tc.Equals, admin1Name)
	c.Check(users[0].DisplayName, tc.Equals, "admin1")
	c.Check(users[0].CreatorUUID, tc.Equals, admin1UUID)
	c.Check(users[0].CreatorName, tc.Equals, admin1Name)
	c.Check(users[0].CreatedAt, tc.NotNil)
	c.Check(users[0].LastLogin, tc.NotNil)
	c.Check(users[0].Disabled, tc.Equals, false)
}

// TestUserWithAuthInfo asserts that we can get a user with auth info from the
// database.
func (s *userStateSuite) TestUserWithAuthInfo(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	uuid, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "newguy")

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		c.Context(),
		uuid,
		name, name.Name(),
		uuid,
		s.controllerLoginAccess(),
		"passwordHash", salt)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DisableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	u, err := st.GetUser(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(u.Name, tc.Equals, name)
	c.Check(u.DisplayName, tc.Equals, name.Name())
	c.Check(u.CreatorUUID, tc.Equals, uuid)
	c.Check(u.CreatedAt, tc.NotNil)
	c.Check(u.LastLogin, tc.NotNil)
	c.Check(u.Disabled, tc.Equals, true)
}

// TestSetPasswordHash asserts that we can set a password hash for a user.
func (s *userStateSuite) TestSetPasswordHash(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	newActivationKey, err := generateActivationKey()
	c.Assert(err, tc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		newActivationKey,
	)
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Set password hash.
	err = st.SetPasswordHash(c.Context(), name, "passwordHash", salt)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	rowAuth := db.QueryRow(`
SELECT disabled
FROM user_authentication
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(rowAuth.Err(), tc.ErrorIsNil)

	var disabled bool
	err = rowAuth.Scan(&disabled)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(disabled, tc.Equals, false)

	row := db.QueryRow(`
SELECT password_hash
FROM user_password
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var passwordHash string
	err = row.Scan(&passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(passwordHash, tc.Equals, "passwordHash")

	row = db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
}

// TestSetPasswordHash asserts that we can set a password hash for a user twice.
func (s *userStateSuite) TestSetPasswordHashTwice(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	newActivationKey, err := generateActivationKey()
	c.Assert(err, tc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		newActivationKey,
	)
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Set password hash.
	err = st.SetPasswordHash(c.Context(), name, "passwordHash", salt)
	c.Assert(err, tc.ErrorIsNil)

	// Set password hash again
	err = st.SetPasswordHash(c.Context(), name, "passwordHashAgain", salt)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT password_hash
FROM user_password
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var passwordHash string
	err = row.Scan(&passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(passwordHash, tc.Equals, "passwordHashAgain")
}

// TestAddUserWithPasswordHash asserts that we can add a user with a password
// hash.
func (s *userStateSuite) TestAddUserWithPasswordHash(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	row := db.QueryRow(`SELECT password_hash FROM user_password WHERE user_uuid = ?`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var passwordHash string
	err = row.Scan(&passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(passwordHash, tc.Equals, "passwordHash")
}

// TestAddUserWithPasswordWhichCreatorDoesNotExist asserts that we get an error
// when we try to add a user with a password that has a creator that does not
// exist.
func (s *userStateSuite) TestAddUserWithPasswordWhichCreatorDoesNotExist(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	nonExistedCreatorUuid, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Try and add admin user with a creator that does not exist.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		nonExistedCreatorUuid,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestAddUserWithActivationKey asserts that we can add a user with an
// activation key.
func (s *userStateSuite) TestAddUserWithActivationKey(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	adminActivationKey, err := generateActivationKey()
	c.Assert(err, tc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		adminActivationKey,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the activation key was set correctly.
	activationKey, err := st.GetActivationKey(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(activationKey, tc.DeepEquals, adminActivationKey)
}

// TestGetActivationKeyNotFound asserts that if we try to get an activation key
// for a user that does not exist, we get an error.
func (s *userStateSuite) TestGetActivationKeyNotFound(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		c.Context(), adminUUID,
		name, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the activation key was set correctly.
	_, err = st.GetActivationKey(c.Context(), name)
	c.Assert(err, tc.ErrorIs, usererrors.ActivationKeyNotFound)
}

// TestAddUserWithActivationKeyWhichCreatorDoesNotExist asserts that we get an
// error when we try to add a user with an activation key that has a creator
// that does not exist.
func (s *userStateSuite) TestAddUserWithActivationKeyWhichCreatorDoesNotExist(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	nonExistedCreatorUuid, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	// Try and add admin user with an activation key with a creator that does not exist.
	newActivationKey, err := generateActivationKey()
	c.Assert(err, tc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		c.Context(), adminUUID,
		name, "admin",
		nonExistedCreatorUuid,
		s.controllerLoginAccess(),
		newActivationKey,
	)
	c.Assert(err, tc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestSetActivationKey asserts that we can set an activation key for a user.
func (s *userStateSuite) TestSetActivationKey(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Set activation key.
	adminActivationKey, err := generateActivationKey()
	c.Assert(err, tc.ErrorIsNil)
	err = st.SetActivationKey(c.Context(), name, adminActivationKey)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the activation key was set correctly, and the password hash was removed.
	db := s.DB()

	row := db.QueryRow(`
SELECT activation_key
FROM user_activation_key
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var activationKey string
	err = row.Scan(&activationKey)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(activationKey, tc.Equals, string(adminActivationKey))

	row = db.QueryRow(`
SELECT password_hash, password_salt
FROM user_password
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var passwordHash, passwordSalt string
	err = row.Scan(&passwordHash, &passwordSalt)
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)
}

// TestDisableUserAuthentication asserts that we can disable a user.
func (s *userStateSuite) TestDisableUserAuthentication(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Disable user.
	err = st.DisableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the user was disabled correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT disabled
FROM user_authentication
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var disabled bool
	err = row.Scan(&disabled)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(disabled, tc.Equals, true)
}

// TestEnableUserAuthentication asserts that we can enable a user.
func (s *userStateSuite) TestEnableUserAuthentication(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		c.Context(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Disable user.
	err = st.DisableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// Enable user.
	err = st.EnableUserAuthentication(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the user was enabled correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT disabled
FROM user_authentication
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var disabled bool
	err = row.Scan(&disabled)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(disabled, tc.Equals, false)
}

func (s *userStateSuite) TestGetUserUUIDByName(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	uuid, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	name := usertesting.GenNewName(c, "dnuof")
	err = st.AddUserWithPermission(
		c.Context(),
		uuid,
		name, "",
		false,
		uuid,
		s.controllerLoginAccess(),
	)
	c.Assert(err, tc.ErrorIsNil)

	gotUUID, err := st.GetUserUUIDByName(c.Context(), name)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, uuid)
}

// TestGetUserUUIDByNameNotFound is asserting that if try and find the uuid for
// a user that doesn't exist we get back a [usererrors.NotFound] error.
func (s *userStateSuite) TestGetUserUUIDByNameNotFound(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	_, err := st.GetUserUUIDByName(c.Context(), usertesting.GenNewName(c, "tlm"))
	c.Check(err, tc.ErrorIs, usererrors.UserNotFound)
}

// TestUpdateLastModelLogin asserts that the model_last_login table is updated
// with the last login time to the model on UpdateLastModelLogin.
func (s *userStateSuite) TestUpdateLastModelLogin(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-update-last-login-model")
	st := NewUserState(s.TxnRunnerFactory())
	name, adminUUID := s.addTestUser(c, st, "admin")
	loginTime := time.Now()

	// Update last login.
	err := st.UpdateLastModelLogin(c.Context(), name, modelUUID, loginTime)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the last login was updated correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT user_uuid, model_uuid, time
FROM model_last_login
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var lastLogin time.Time
	var dbModelUUID string
	var dbUserUUID string
	err = row.Scan(&dbUserUUID, &dbModelUUID, &lastLogin)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(lastLogin.UTC(), tc.Equals, loginTime.Truncate(time.Second).UTC())
	c.Assert(dbUserUUID, tc.Equals, string(adminUUID))
	c.Assert(dbModelUUID, tc.Equals, string(modelUUID))
}

func (s *userStateSuite) TestUpdateLastModelLoginModelNotFound(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	name, _ := s.addTestUser(c, st, "admin")
	badModelUUID, err := coremodel.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	// Update last login.
	err = st.UpdateLastModelLogin(c.Context(), name, badModelUUID, time.Time{})
	c.Assert(err, tc.ErrorMatches, ".*model not found.*")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *userStateSuite) TestLastModelLogin(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-last-model-login")
	st := NewUserState(s.TxnRunnerFactory())
	username1, _ := s.addTestUser(c, st, "user1")
	username2, _ := s.addTestUser(c, st, "user2")
	expectedTime1 := time.Now()
	expectedTime2 := expectedTime1.Add(time.Minute)

	// Simulate two logins to the model.
	err := st.UpdateLastModelLogin(c.Context(), username1, modelUUID, expectedTime1)
	c.Assert(err, tc.ErrorIsNil)
	err = st.UpdateLastModelLogin(c.Context(), username2, modelUUID, expectedTime2)
	c.Assert(err, tc.ErrorIsNil)

	// Check login times.
	time1, err := st.LastModelLogin(c.Context(), username1, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(time1.UTC(), tc.Equals, expectedTime1.Truncate(time.Second).UTC())
	time2, err := st.LastModelLogin(c.Context(), username2, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(time2.UTC(), tc.Equals, expectedTime2.Truncate(time.Second).UTC())

	// Simulate a new login from user1
	expectedTime3 := expectedTime2.Add(time.Minute)
	err = st.UpdateLastModelLogin(c.Context(), username1, modelUUID, expectedTime3)
	c.Assert(err, tc.ErrorIsNil)

	// Check the time for user1 was updated.
	time3, err := st.LastModelLogin(c.Context(), username1, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(time3, tc.Equals, expectedTime3.Truncate(time.Second).UTC())
}

func (s *userStateSuite) TestLastModelLoginModelNotFound(c *tc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	name, _ := s.addTestUser(c, st, "admin")
	badModelUUID, err := coremodel.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	// Get users last login for non existent model.
	_, err = st.LastModelLogin(c.Context(), name, badModelUUID)
	c.Assert(err, tc.ErrorMatches, ".*model not found.*")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *userStateSuite) TestLastModelLoginModelUserNeverAccessedModel(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-last-model-login")
	st := NewUserState(s.TxnRunnerFactory())
	name, _ := s.addTestUser(c, st, "admin")

	// Get users last login for non existent model.
	_, err := st.LastModelLogin(c.Context(), name, modelUUID)
	c.Assert(err, tc.ErrorIs, usererrors.UserNeverAccessedModel)
}

func (s *userStateSuite) addTestUser(c *tc.C, st *UserState, name string) (user.Name, user.UUID) {
	userUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, tc.ErrorIsNil)

	userName := usertesting.GenNewName(c, name)
	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		c.Context(), userUUID,
		userName, name,
		userUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, tc.ErrorIsNil)
	return userName, userUUID
}

func (s *userStateSuite) controllerLoginAccess() permission.AccessSpec {
	return permission.AccessSpec{
		Access: permission.LoginAccess,
		Target: permission.ID{
			ObjectType: permission.Controller,
			Key:        s.controllerUUID,
		},
	}
}
