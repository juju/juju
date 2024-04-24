// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// user_model_test.go contains tests that need a model in state. The function
// CreateTestModel from model/testing imports access so using it in the tests in
// the state package creates a cyclical dependency.
package state_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/state"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/auth"
)

type userModelStateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&userModelStateSuite{})

// TestUpdateLastLoginUserAuth asserts that the user_authentication table is
// updated with the last login time on UpdateLastLogin.
func (s *userModelStateSuite) TestUpdateLastLoginUserAuth(c *gc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-update-last-login-user-auth")
	st := state.NewUserState(s.TxnRunnerFactory())
	name, adminUUID := s.addTestUser(c, st, "admin")

	// Update last login.
	err := st.UpdateLastLogin(context.Background(), modelUUID, name)
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

// TestUpdateLastLoginModel asserts that the model_last_login table is updated
// with the last login time to the model on UpdateLastLogin.
func (s *userModelStateSuite) TestUpdateLastLoginModel(c *gc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-update-last-login-model")
	st := state.NewUserState(s.TxnRunnerFactory())
	name, adminUUID := s.addTestUser(c, st, "admin")

	// Update last login.
	err := st.UpdateLastLogin(context.Background(), modelUUID, name)
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

	c.Assert(lastLogin, gc.NotNil)
	c.Assert(dbUserUUID, gc.Equals, string(adminUUID))
	c.Assert(dbModelUUID, gc.Equals, string(modelUUID))
}

// TestUpdateLastLoginModel asserts that the model_last_login table is updated
// with the last login time to the model on UpdateLastLogin.
func (s *userModelStateSuite) TestUpdateLastLoginModelNotFound(c *gc.C) {
	st := state.NewUserState(s.TxnRunnerFactory())
	name, _ := s.addTestUser(c, st, "admin")
	badModelUUID, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	// Update last login.
	err = st.UpdateLastLogin(context.Background(), badModelUUID, name)
	c.Assert(err, gc.ErrorMatches, ".*model not found.*")
}

func (s *userModelStateSuite) TestLastModelConnection(c *gc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-last-model-login")
	st := state.NewUserState(s.TxnRunnerFactory())
	username1, _ := s.addTestUser(c, st, "user1")
	username2, _ := s.addTestUser(c, st, "user2")

	// Simulate two logins to the model.
	err := st.UpdateLastLogin(context.Background(), modelUUID, username1)
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpdateLastLogin(context.Background(), modelUUID, username2)
	c.Assert(err, jc.ErrorIsNil)

	// Check user2 was the last to login.
	time1, err := st.LastModelConnection(context.Background(), modelUUID, username1)
	c.Assert(err, jc.ErrorIsNil)
	time2, err := st.LastModelConnection(context.Background(), modelUUID, username2)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(time1.Before(time2), jc.IsTrue, gc.Commentf("time1 is after time2 (%s is after %s)", time1, time2))
	// Simluate a new login from user1
	err = st.UpdateLastLogin(context.Background(), modelUUID, username1)
	c.Assert(err, jc.ErrorIsNil)

	// Check the time for user1 was updated.
	time1, err = st.LastModelConnection(context.Background(), modelUUID, username1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(time2.Before(time1), jc.IsTrue)
}

func (s *userModelStateSuite) addTestUser(c *gc.C, st *state.UserState, name string) (string, user.UUID) {
	// Add admin user with activation key.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	salt, err := auth.NewSalt()
	c.Assert(err, jc.ErrorIsNil)

	// Add user with password hash.
	err = st.AddUserWithPasswordHash(
		context.Background(), userUUID,
		name, name,
		userUUID,
		controllerLoginAccess(),
		"passwordHash", salt,
	)
	c.Assert(err, jc.ErrorIsNil)
	return name, userUUID
}

func controllerLoginAccess() permission.AccessSpec {
	return permission.AccessSpec{
		Access: permission.LoginAccess,
		Target: permission.ID{
			ObjectType: permission.Controller,
			Key:        coredatabase.ControllerNS,
		},
	}
}
