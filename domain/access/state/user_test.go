// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"crypto/rand"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&userStateSuite{})

func (s *userStateSuite) SetUpTest(c *gc.C) {
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
func (s *userStateSuite) TestSingletonActiveUser(c *gc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "123", "bob", "Bob", false, true, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "124", "bob", "Bob", false, true, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "125", "bob", "Bob", false, true, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Insert the first non-removed (active) Bob user.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "126", "bob", "Bob", false, false, "123", time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	// Try and insert the second non-removed (active) Bob user. This should blow
	// up the constraint.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, "127", "bob", "Bob", false, false, "123", time.Now())
		return err
	})
	c.Assert(database.IsErrConstraintUnique(err), jc.IsTrue)
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
func (s *userStateSuite) TestBootstrapAddUserWithPassword(c *gc.C) {
	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with no password authorization.
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err = AddUserWithPassword(
			context.Background(), tx, adminUUID,
			usertesting.GenNewName(c, "admin"), "admin",
			adminUUID, s.controllerLoginAccess(), "passwordHash", salt,
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

// TestAddUser asserts a new user is added, enabled, and has
// the provided permission.
func (s *userStateSuite) TestAddUser(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		context.Background(), adminUUID,
		name, "admin", false,
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	newUser, err := st.GetUser(context.Background(), adminUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newUser.Name, gc.Equals, name)
	c.Check(newUser.UUID, gc.Equals, adminUUID)
	c.Check(newUser.Disabled, jc.IsFalse)
	c.Check(newUser.CreatorUUID, gc.Equals, adminUUID)
}

// TestAddUserAlreadyExists asserts that we get an error when we try to add a
// user that already exists.
func (s *userStateSuite) TestAddUserAlreadyExists(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		context.Background(), adminUUID,
		name, "admin", false,
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user again.
	adminCloneUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = st.AddUser(
		context.Background(), adminCloneUUID,
		name, "admin", false,
		adminCloneUUID,
	)
	c.Assert(err, jc.ErrorIs, usererrors.UserAlreadyExists)
}

// TestAddUserCreatorNotFound asserts that we get an error when we try
// to add a user that has a creator that does not exist.
func (s *userStateSuite) TestAddUserCreatorNotFound(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user with a creator that does not exist.
	nonExistingUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		context.Background(), adminUUID,
		name, "admin", false,
		nonExistingUUID,
	)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestAddUserWithPermission asserts a new user is added, enabled, and has
// the provided permission.
func (s *userStateSuite) TestAddUserWithPermission(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	loginAccess := s.controllerLoginAccess()
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		context.Background(), adminUUID,
		name, "admin", false,
		adminUUID, loginAccess,
	)
	c.Assert(err, jc.ErrorIsNil)

	newUser, err := st.GetUser(context.Background(), adminUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newUser.Name, gc.Equals, name)
	c.Check(newUser.UUID, gc.Equals, adminUUID)
	c.Check(newUser.Disabled, jc.IsFalse)
	c.Check(newUser.CreatorUUID, gc.Equals, adminUUID)
	c.Check(newUser.CreatorName, gc.Equals, user.AdminUserName)

	pSt := NewPermissionState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	newUserAccess, err := pSt.ReadUserAccessForTarget(context.Background(), usertesting.GenNewName(c, "admin"), loginAccess.Target)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newUserAccess.Access, gc.Equals, loginAccess.Access)
	c.Check(newUserAccess.UserName, gc.Equals, newUser.Name)
	c.Check(newUserAccess.Object, gc.Equals, loginAccess.Target)
}

// TestAddUserWithPermissionInvalid asserts that we can't add a user to the
// database.
func (s *userStateSuite) TestAddUserWithPermissionInvalid(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
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
	c.Assert(err, jc.ErrorIs, usererrors.PermissionTargetInvalid)
}

// TestGetUser asserts that we can get a user from the database.
func (s *userStateSuite) TestGetUser(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUser(context.Background(), adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatorName, gc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, gc.NotNil)
}

// TestGetRemovedUser asserts that we can get a removed user from the database.
func (s *userStateSuite) TestGetRemovedUser(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		context.Background(), adminUUID,
		adminName, "admin", false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUserWithPasswordHash(
		context.Background(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), userToRemoveName)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUser(context.Background(), userToRemoveUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, userToRemoveName)
	c.Check(u.DisplayName, gc.Equals, "userToRemove")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatorName, gc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, gc.NotNil)
}

// TestGetUserNotFound asserts that we get an error when we try to get a user
// that does not exist.
func (s *userStateSuite) TestGetUserNotFound(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Generate a random UUID.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUser(context.Background(), userUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUserByName asserts that we can get a user by name from the database.
func (s *userStateSuite) TestGetUserByName(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatorName, gc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.LastLogin, gc.NotNil)
	c.Check(u.Disabled, gc.Equals, false)
}

// TestGetRemovedUserByName asserts that we can get only non-removed user by name.
func (s *userStateSuite) TestGetRemovedUserByName(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		context.Background(), adminUUID,
		adminName, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUserWithPermission(
		context.Background(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), userToRemoveName)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByName(context.Background(), userToRemoveName)
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUserByNameMultipleUsers asserts that we get a non-removed user when we try to
// get a user by name that has multiple users with the same name.
func (s *userStateSuite) TestGetUserByNameMultipleUsers(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		context.Background(), adminUUID,
		name, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove admin user.
	err = st.RemoveUser(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin2 user.
	admin2UUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(),
		admin2UUID,
		name, "admin2",
		admin2UUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, "admin2")
	c.Check(u.CreatorUUID, gc.Equals, admin2UUID)
	c.Check(u.CreatorName, gc.Equals, name)
	c.Check(u.CreatedAt, gc.NotNil)
}

// TestGetUserByNameNotFound asserts that we get an error when we try to get a
// user by name that does not exist.
func (s *userStateSuite) TestGetUserByNameNotFound(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Get the user.
	_, err := st.GetUserByName(context.Background(), usertesting.GenNewName(c, "admin"))
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUserWithAuthInfoByName asserts that we can get a user with auth info
// by name from the database.
func (s *userStateSuite) TestGetUserWithAuthInfoByName(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByName(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatorName, gc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.LastLogin, gc.NotNil)
	c.Check(u.Disabled, gc.Equals, false)
}

// TestGetUserByAuth asserts that we can get a user by auth from the database.
func (s *userStateSuite) TestGetUserByAuth(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(),
		adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		passwordHash, salt)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByAuth(context.Background(), name, auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatorName, gc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.Disabled, jc.IsFalse)
}

// TestGetUserByAuthWithInvalidSalt asserts that we correctly send an
// unauthorized error if the user doesn't have a valid salt.
func (s *userStateSuite) TestGetUserByAuthWithInvalidSalt(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", []byte{},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByAuth(context.Background(), name, auth.NewPassword("passwordHash"))
	c.Assert(err, jc.ErrorIs, usererrors.UserUnauthorized)
}

// TestGetUserByAuthDisabled asserts that we can get a user by auth from the
// database and has the correct disabled flag.
func (s *userStateSuite) TestGetUserByAuthDisabled(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(),
		adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		passwordHash, salt)
	c.Assert(err, jc.ErrorIsNil)

	err = st.DisableUserAuthentication(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	u, err := st.GetUserByAuth(context.Background(), name, auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, "admin")
	c.Check(u.CreatorUUID, gc.Equals, adminUUID)
	c.Check(u.CreatorName, gc.Equals, user.AdminUserName)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.Disabled, jc.IsTrue)
}

// TestGetUserByAuthUnauthorized asserts that we get an error when we try to
// get a user by auth with the wrong password.
func (s *userStateSuite) TestGetUserByAuthUnauthorized(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with password hash.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash, err := auth.HashPassword(auth.NewPassword("password"), salt)
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		passwordHash, salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Get the user.
	_, err = st.GetUserByAuth(context.Background(), name, auth.NewPassword("wrong"))
	c.Assert(err, jc.ErrorIs, usererrors.UserUnauthorized)
}

// TestGetUserByAuthDoesNotExist asserts that we get an error when we try to
// get a user by auth that does not exist.
func (s *userStateSuite) TestGetUserByAuthDoesNotExist(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Get the user.
	_, err := st.GetUserByAuth(context.Background(), usertesting.GenNewName(c, "admin"), auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestRemoveUser asserts that we can remove a user from the database.
func (s *userStateSuite) TestRemoveUser(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		context.Background(), adminUUID,
		adminName, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUserWithPermission(
		context.Background(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), userToRemoveName)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(row.Scan(nil), jc.ErrorIs, sql.ErrNoRows)

	// Check that the user activation key was removed
	row = db.QueryRow(`
SELECT user_uuid
FROM user_activation_key
WHERE user_uuid = ?
	`, userToRemoveUUID)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), jc.ErrorIs, sql.ErrNoRows)

	// Check that the user was marked as removed.
	row = db.QueryRow(`
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

// TestRemoveUserSSHKeys is here to test that when we remove a user from the
// Juju database we delete all ssh keys for the user.
func (s *userStateSuite) TestRemoveUserSSHKeys(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	adminName := usertesting.GenNewName(c, "admin")
	err = st.AddUser(
		context.Background(), adminUUID,
		adminName, "admin",
		false,
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add userToRemove.
	userToRemoveUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	userToRemoveName := usertesting.GenNewName(c, "userToRemove")
	err = st.AddUser(
		context.Background(), userToRemoveUUID,
		userToRemoveName, "userToRemove",
		false,
		adminUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	modelId := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")

	// Add a public key onto a model for the user.
	km := keymanagerstate.NewState(s.TxnRunnerFactory())
	err = km.AddPublicKeysForUser(context.Background(), modelId, userToRemoveUUID, []keymanager.PublicKey{
		{
			Comment:         "test",
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     "something",
			Key:             "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju2@example.com",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Remove userToRemove.
	err = st.RemoveUser(context.Background(), userToRemoveName)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the user has been successfully removed.
	db := s.DB()

	row := db.QueryRow(`
SELECT model_uuid
FROM model_authorized_keys
`)
	c.Assert(row.Scan(nil), jc.ErrorIs, sql.ErrNoRows)

	row = db.QueryRow(`
SELECT id
FROM user_public_ssh_key
WHERE user_uuid = ?
`, userToRemoveUUID)
	c.Assert(row.Scan(nil), jc.ErrorIs, sql.ErrNoRows)

	// Check that the user password was removed
	row = db.QueryRow(`
SELECT user_uuid
FROM user_password
WHERE user_uuid = ?
	`, userToRemoveUUID)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), jc.ErrorIs, sql.ErrNoRows)

	// Check that the user activation key was removed
	row = db.QueryRow(`
SELECT user_uuid
FROM user_activation_key
WHERE user_uuid = ?
	`, userToRemoveUUID)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), jc.ErrorIs, sql.ErrNoRows)

	// Check that the user was marked as removed.
	row = db.QueryRow(`
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
func (s *userStateSuite) TestGetAllUsersWihAuthInfo(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin1 user with password hash.
	admin1UUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	admin1Name := usertesting.GenNewName(c, "admin1")
	err = st.AddUserWithPasswordHash(
		context.Background(), admin1UUID,
		admin1Name, "admin1",
		admin1UUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add admin2 user with activation key.
	admin2UUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	admin2ActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)

	admin2Name := usertesting.GenNewName(c, "admin2")
	err = st.AddUserWithActivationKey(
		context.Background(), admin2UUID,
		admin2Name, "admin2",
		admin2UUID,
		s.controllerLoginAccess(),
		admin2ActivationKey,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Disable admin2 user.
	err = st.DisableUserAuthentication(context.Background(), admin2Name)
	c.Assert(err, jc.ErrorIsNil)

	// Get all users with auth info, including disabled users.
	users, err := st.GetAllUsers(context.Background(), true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(users, gc.HasLen, 2)

	c.Check(users[0].Name, gc.Equals, admin1Name)
	c.Check(users[0].DisplayName, gc.Equals, "admin1")
	c.Check(users[0].CreatorUUID, gc.Equals, admin1UUID)
	c.Check(users[0].CreatorName, gc.Equals, admin1Name)
	c.Check(users[0].CreatedAt, gc.NotNil)
	c.Check(users[0].LastLogin, gc.NotNil)
	c.Check(users[0].Disabled, gc.Equals, false)

	c.Check(users[1].Name, gc.Equals, admin2Name)
	c.Check(users[1].DisplayName, gc.Equals, "admin2")
	c.Check(users[1].CreatorUUID, gc.Equals, admin2UUID)
	c.Check(users[1].CreatorName, gc.Equals, admin2Name)
	c.Check(users[1].CreatedAt, gc.NotNil)
	c.Check(users[1].LastLogin, gc.NotNil)
	c.Check(users[1].Disabled, gc.Equals, true)

	// Get all users with auth info, excluding disabled users
	users, err = st.GetAllUsers(context.Background(), false)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(users, gc.HasLen, 1)

	c.Check(users[0].Name, gc.Equals, admin1Name)
	c.Check(users[0].DisplayName, gc.Equals, "admin1")
	c.Check(users[0].CreatorUUID, gc.Equals, admin1UUID)
	c.Check(users[0].CreatorName, gc.Equals, admin1Name)
	c.Check(users[0].CreatedAt, gc.NotNil)
	c.Check(users[0].LastLogin, gc.NotNil)
	c.Check(users[0].Disabled, gc.Equals, false)
}

// TestUserWithAuthInfo asserts that we can get a user with auth info from the
// database.
func (s *userStateSuite) TestUserWithAuthInfo(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "newguy")

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	err = st.AddUserWithPasswordHash(
		context.Background(),
		uuid,
		name, name.Name(),
		uuid,
		s.controllerLoginAccess(),
		"passwordHash", salt)
	c.Assert(err, jc.ErrorIsNil)

	err = st.DisableUserAuthentication(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	u, err := st.GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(u.Name, gc.Equals, name)
	c.Check(u.DisplayName, gc.Equals, name.Name())
	c.Check(u.CreatorUUID, gc.Equals, uuid)
	c.Check(u.CreatedAt, gc.NotNil)
	c.Check(u.LastLogin, gc.NotNil)
	c.Check(u.Disabled, gc.Equals, true)
}

// TestSetPasswordHash asserts that we can set a password hash for a user.
func (s *userStateSuite) TestSetPasswordHash(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	newActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		newActivationKey,
	)
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Set password hash.
	err = st.SetPasswordHash(context.Background(), name, "passwordHash", salt)
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

// TestSetPasswordHash asserts that we can set a password hash for a user twice.
func (s *userStateSuite) TestSetPasswordHashTwice(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	newActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		newActivationKey,
	)
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Set password hash.
	err = st.SetPasswordHash(context.Background(), name, "passwordHash", salt)
	c.Assert(err, jc.ErrorIsNil)

	// Set password hash again
	err = st.SetPasswordHash(context.Background(), name, "passwordHashAgain", salt)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the password hash was set correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT password_hash
FROM user_password
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var passwordHash string
	err = row.Scan(&passwordHash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(passwordHash, gc.Equals, "passwordHashAgain")
}

// TestAddUserWithPasswordHash asserts that we can add a user with a password
// hash.
func (s *userStateSuite) TestAddUserWithPasswordHash(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
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
func (s *userStateSuite) TestAddUserWithPasswordWhichCreatorDoesNotExist(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	nonExistedCreatorUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user with a creator that does not exist.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		nonExistedCreatorUuid,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestAddUserWithActivationKey asserts that we can add a user with an
// activation key.
func (s *userStateSuite) TestAddUserWithActivationKey(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	adminActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		adminActivationKey,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the activation key was set correctly.
	activationKey, err := st.GetActivationKey(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(activationKey, gc.DeepEquals, adminActivationKey)
}

// TestGetActivationKeyNotFound asserts that if we try to get an activation key
// for a user that does not exist, we get an error.
func (s *userStateSuite) TestGetActivationKeyNotFound(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPermission(
		context.Background(), adminUUID,
		name, "admin",
		false,
		adminUUID,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the activation key was set correctly.
	_, err = st.GetActivationKey(context.Background(), name)
	c.Assert(err, jc.ErrorIs, usererrors.ActivationKeyNotFound)
}

// TestAddUserWithActivationKeyWhichCreatorDoesNotExist asserts that we get an
// error when we try to add a user with an activation key that has a creator
// that does not exist.
func (s *userStateSuite) TestAddUserWithActivationKeyWhichCreatorDoesNotExist(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	nonExistedCreatorUuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Try and add admin user with an activation key with a creator that does not exist.
	newActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithActivationKey(
		context.Background(), adminUUID,
		name, "admin",
		nonExistedCreatorUuid,
		s.controllerLoginAccess(),
		newActivationKey,
	)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestSetActivationKey asserts that we can set an activation key for a user.
func (s *userStateSuite) TestSetActivationKey(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Set activation key.
	adminActivationKey, err := generateActivationKey()
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetActivationKey(context.Background(), name, adminActivationKey)
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
func (s *userStateSuite) TestDisableUserAuthentication(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Disable user.
	err = st.DisableUserAuthentication(context.Background(), name)
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
func (s *userStateSuite) TestEnableUserAuthentication(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())

	// Add admin user with activation key.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	name := usertesting.GenNewName(c, "admin")
	err = st.AddUserWithPasswordHash(
		context.Background(), adminUUID,
		name, "admin",
		adminUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Disable user.
	err = st.DisableUserAuthentication(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)

	// Enable user.
	err = st.EnableUserAuthentication(context.Background(), name)
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

func (s *userStateSuite) TestGetUserUUIDByName(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	name := usertesting.GenNewName(c, "dnuof")
	err = st.AddUserWithPermission(
		context.Background(),
		uuid,
		name, "",
		false,
		uuid,
		s.controllerLoginAccess(),
	)
	c.Assert(err, jc.ErrorIsNil)

	gotUUID, err := st.GetUserUUIDByName(context.Background(), name)
	c.Check(err, jc.ErrorIsNil)
	c.Check(gotUUID, gc.Equals, uuid)
}

// TestGetUserUUIDByNameNotFound is asserting that if try and find the uuid for
// a user that doesn't exist we get back a [usererrors.NotFound] error.
func (s *userStateSuite) TestGetUserUUIDByNameNotFound(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	_, err := st.GetUserUUIDByName(context.Background(), usertesting.GenNewName(c, "tlm"))
	c.Check(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestUpdateLastModelLogin asserts that the model_last_login table is updated
// with the last login time to the model on UpdateLastModelLogin.
func (s *userStateSuite) TestUpdateLastModelLogin(c *gc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-update-last-login-model")
	st := NewUserState(s.TxnRunnerFactory())
	name, adminUUID := s.addTestUser(c, st, "admin")
	loginTime := time.Now()

	// Update last login.
	err := st.UpdateLastModelLogin(context.Background(), name, modelUUID, loginTime)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the last login was updated correctly.
	db := s.DB()

	row := db.QueryRow(`
SELECT user_uuid, model_uuid, time
FROM model_last_login
WHERE user_uuid = ?
	`, adminUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var lastLogin time.Time
	var dbModelUUID string
	var dbUserUUID string
	err = row.Scan(&dbUserUUID, &dbModelUUID, &lastLogin)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(lastLogin.UTC(), gc.Equals, loginTime.Truncate(time.Second).UTC())
	c.Assert(dbUserUUID, gc.Equals, string(adminUUID))
	c.Assert(dbModelUUID, gc.Equals, string(modelUUID))
}

func (s *userStateSuite) TestUpdateLastModelLoginModelNotFound(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	name, _ := s.addTestUser(c, st, "admin")
	badModelUUID, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Update last login.
	err = st.UpdateLastModelLogin(context.Background(), name, badModelUUID, time.Time{})
	c.Assert(err, gc.ErrorMatches, ".*model not found.*")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *userStateSuite) TestLastModelLogin(c *gc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-last-model-login")
	st := NewUserState(s.TxnRunnerFactory())
	username1, _ := s.addTestUser(c, st, "user1")
	username2, _ := s.addTestUser(c, st, "user2")
	expectedTime1 := time.Now()
	expectedTime2 := expectedTime1.Add(time.Minute)

	// Simulate two logins to the model.
	err := st.UpdateLastModelLogin(context.Background(), username1, modelUUID, expectedTime1)
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpdateLastModelLogin(context.Background(), username2, modelUUID, expectedTime2)
	c.Assert(err, jc.ErrorIsNil)

	// Check login times.
	time1, err := st.LastModelLogin(context.Background(), username1, modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(time1.UTC(), gc.Equals, expectedTime1.Truncate(time.Second).UTC())
	time2, err := st.LastModelLogin(context.Background(), username2, modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(time2.UTC(), gc.Equals, expectedTime2.Truncate(time.Second).UTC())

	// Simulate a new login from user1
	expectedTime3 := expectedTime2.Add(time.Minute)
	err = st.UpdateLastModelLogin(context.Background(), username1, modelUUID, expectedTime3)
	c.Assert(err, jc.ErrorIsNil)

	// Check the time for user1 was updated.
	time3, err := st.LastModelLogin(context.Background(), username1, modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(time3, gc.Equals, expectedTime3.Truncate(time.Second).UTC())
}

func (s *userStateSuite) TestLastModelLoginModelNotFound(c *gc.C) {
	st := NewUserState(s.TxnRunnerFactory())
	name, _ := s.addTestUser(c, st, "admin")
	badModelUUID, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Get users last login for non existent model.
	_, err = st.LastModelLogin(context.Background(), name, badModelUUID)
	c.Assert(err, gc.ErrorMatches, ".*model not found.*")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *userStateSuite) TestLastModelLoginModelUserNeverAccessedModel(c *gc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-last-model-login")
	st := NewUserState(s.TxnRunnerFactory())
	name, _ := s.addTestUser(c, st, "admin")

	// Get users last login for non existent model.
	_, err := st.LastModelLogin(context.Background(), name, modelUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UserNeverAccessedModel)
}

func (s *userStateSuite) addTestUser(c *gc.C, st *UserState, name string) (user.Name, user.UUID) {
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	userName := usertesting.GenNewName(c, name)
	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		context.Background(), userUUID,
		userName, name,
		userUUID,
		s.controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)
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
