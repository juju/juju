// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	domainuser "github.com/juju/juju/domain/user"
	usererrors "github.com/juju/juju/domain/user/errors"
	usertesting "github.com/juju/juju/domain/user/testing"
	"github.com/juju/juju/internal/auth"
)

type serviceSuite struct {
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state)
}

// TestAddUserNameNotValid is testing that if we try and add a user with a
// username that is not valid we get an error that satisfies
// usererrors.UserNameNotValid back.
func (s *serviceSuite) TestAddUserNameNotValid(c *gc.C) {
	_, _, err := s.service().AddUser(context.Background(), AddUserArg{Name: usertesting.InvalidUsernames[0]})
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TODO (manadart): Add a matcher to this test that ensures we still get a UUID
// when none was supplied.

// TestAddUserAlreadyExists is testing that we cannot add a user with a username
// that already exists and is active. We expect that in this case we should
// receive an error back that satisfies usererrors.AlreadyExists.
func (s *serviceSuite) TestAddUserAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The matcher used below verifies that we generated a
	// UUID when one was not suppied in the AddUserArg.
	a := gomock.Any()
	s.state.EXPECT().AddUserWithActivationKey(a, stringerNotEmpty{}, a, a, a, a).Return(usererrors.AlreadyExists)

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		Name:        "valid",
		CreatorUUID: mustNewUUID(),
	})
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)
}

func (s *serviceSuite) TestAddUserCreatorUUIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The matcher used below verifies that we generated a
	// UUID when one was not supplied in the AddUserArg.
	a := gomock.Any()
	s.state.EXPECT().AddUserWithActivationKey(a, stringerNotEmpty{}, a, a, a, a).Return(usererrors.CreatorUUIDNotFound)

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		Name:        "valid",
		CreatorUUID: mustNewUUID(),
	})
	c.Assert(err, jc.ErrorIs, usererrors.CreatorUUIDNotFound)
}

// TestAddUserWithPassword is testing the happy path of addUserWithPassword.
func (s *serviceSuite) TestAddUserWithPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	userUUID := mustNewUUID()
	creatorUUID := mustNewUUID()

	s.state.EXPECT().AddUserWithPasswordHash(
		gomock.Any(), userUUID, "valid", "display", creatorUUID, gomock.Any(), gomock.Any()).Return(nil)

	pass := auth.NewPassword("password")

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		UUID:        userUUID,
		Name:        "valid",
		DisplayName: "display",
		Password:    &pass,
		CreatorUUID: creatorUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
}

// TestAddUserWithPasswordNotValid is checking that if we try and add a user
// with password that is not valid we get back a error that satisfies
// internal/auth.ErrPasswordNotValid.
func (s *serviceSuite) TestAddUserWithPasswordNotValid(c *gc.C) {
	// This exceeds the maximum password length.
	buff := make([]byte, 2000)
	_, _ = rand.Read(buff)
	badPass := auth.NewPassword(base64.StdEncoding.EncodeToString(buff))

	userUUID := mustNewUUID()
	creatorUUID := mustNewUUID()

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		UUID:        userUUID,
		Name:        "valid",
		DisplayName: "display",
		Password:    &badPass,
		CreatorUUID: creatorUUID,
	})
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)
}

// TestRemoveUser is testing the happy path for removing a user.
func (s *serviceSuite) TestRemoveUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RemoveUser(gomock.Any(), "user").Return(nil)

	err := s.service().RemoveUser(context.Background(), "user")
	c.Assert(err, jc.ErrorIsNil)
}

// TestRemoveUserInvalidUsername is testing that if we supply RemoveUser with
// invalid usernames we get back an error.
func (s *serviceSuite) TestRemoveUserInvalidUsername(c *gc.C) {
	err := s.service().RemoveUser(context.Background(), usertesting.InvalidUsernames[0])
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestRemoveUserNotFound is testing that trying to remove a user that does not
// exist results in an error that satisfies usererrors.NotFound. We also
// check that no state changes occur.
func (s *serviceSuite) TestRemoveUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RemoveUser(gomock.Any(), "missing").Return(usererrors.NotFound)

	err := s.service().RemoveUser(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestSetPassword is testing the happy path for SetPassword.
func (s *serviceSuite) TestSetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	a := gomock.Any()
	s.state.EXPECT().SetPasswordHash(a, a, a, a).Return(nil)

	err := s.service().SetPassword(context.Background(), "user", auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIsNil)
}

// TestSetPasswordInvalidUsername is testing that if we supply SetPassword with
// invalid usernames we get back an error.
func (s *serviceSuite) TestSetPasswordInvalidUsername(c *gc.C) {
	err := s.service().SetPassword(context.Background(), usertesting.InvalidUsernames[0], auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestSetPasswordUserNotFound is testing that when setting a password for a
// user that doesn't exist we get a user.NotFound error.
func (s *serviceSuite) TestSetPasswordUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	a := gomock.Any()
	s.state.EXPECT().SetPasswordHash(a, a, a, a).Return(usererrors.NotFound)

	err := s.service().SetPassword(context.Background(), "user", auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestSetPasswordInvalid is asserting that if pass invalid passwords to
// SetPassword the correct errors are returned.
func (s *serviceSuite) TestSetPasswordInvalid(c *gc.C) {
	err := s.service().SetPassword(context.Background(), "username", auth.NewPassword(""))
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)
}

// TestResetPassword tests the happy path for resetting a user's password.
func (s *serviceSuite) TestResetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetActivationKey(gomock.Any(), "name", gomock.Any()).Return(nil)

	key, err := s.service().ResetPassword(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(key, gc.Not(gc.Equals), "")
}

// TestResetPasswordInvalidUsername is testing that if we supply ResetPassword
// with invalid usernames we get back an error.
func (s *serviceSuite) TestResetPasswordInvalidUsername(c *gc.C) {
	_, err := s.service().ResetPassword(context.Background(), usertesting.InvalidUsernames[0])
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestResetPassword is testing that resting a password for a user that doesn't
// exist returns a usererrors.NotFound error and that no state change occurs.
func (s *serviceSuite) TestResetPasswordUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetActivationKey(gomock.Any(), "name", gomock.Any()).Return(usererrors.NotFound)

	_, err := s.service().ResetPassword(context.Background(), "name")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestGetUserNotFound is testing what the service does when we ask for a user
// that doesn't exist. The expected behaviour is that an error is returned that
// satisfies usererrors.NotFound.
func (s *serviceSuite) TestGetUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustNewUUID()
	s.state.EXPECT().GetUser(gomock.Any(), uuid).Return(user.User{}, usererrors.NotFound)

	_, err := s.service().GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestGetUser is asserting the safe path of GetUser in that if we supply a
// happy and good username and the username exists in state we get back a valid
// user object.
func (s *serviceSuite) TestGetUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustNewUUID()
	s.state.EXPECT().GetUser(gomock.Any(), uuid).Return(user.User{
		UUID: uuid,
		Name: "user",
	}, nil)

	u, err := s.service().GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u.Name, gc.Equals, "user")
}

// TestGetUserByName tests the happy path for GetUserByName.
func (s *serviceSuite) TestGetUserByName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustNewUUID()
	s.state.EXPECT().GetUserByName(gomock.Any(), "name").Return(user.User{
		UUID: uuid,
		Name: "user",
	}, nil)

	u, err := s.service().GetUserByName(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u.UUID, gc.Equals, uuid)
}

// TestGetUserByNameNotFound is testing that if we ask for a user by name that
// doesn't exist we get back an error that satisfies usererrors.NotFound.
func (s *serviceSuite) TestGetUserByNameNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUserByName(gomock.Any(), "user").Return(user.User{}, usererrors.NotFound)

	_, err := s.service().GetUserByName(context.Background(), "user")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestGetAllUsers tests the happy path for GetAllUsers.
func (s *serviceSuite) TestGetAllUsers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUsers(gomock.Any()).Return([]user.User{
		{
			UUID: mustNewUUID(),
			Name: "user0",
		},
		{
			UUID: mustNewUUID(),
			Name: "user1",
		},
	}, nil)

	users, err := s.service().GetAllUsers(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(users, gc.HasLen, 2)
	c.Check(users[0].Name, gc.Equals, "user0")
	c.Check(users[1].Name, gc.Equals, "user1")
}

// TestGetUserByNameInvalidUsername is here to assert that when we ask for a user with
// a username that is invalid we get a UsernameNotValid error. We also check
// here that the service doesn't let invalid usernames flow through to the state
// layer.
func (s *serviceSuite) TestGetUserByNameInvalidUsername(c *gc.C) {
	_, err := s.service().GetUserByName(context.Background(), usertesting.InvalidUsernames[0])
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestGetUserByAuth is testing the happy path for GetUserByAuth.
func (s *serviceSuite) TestGetUserByAuth(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustNewUUID()
	s.state.EXPECT().GetUserByAuth(gomock.Any(), "name", auth.NewPassword("pass")).Return(user.User{
		UUID: uuid,
		Name: "user",
	}, nil)

	u, err := s.service().GetUserByAuth(context.Background(), "name", "pass")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u.UUID, gc.Equals, uuid)
}

// TestEnableUserAuthentication tests the happy path for EnableUserAuthentication.
func (s *serviceSuite) TestEnableUserAuthentication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().EnableUserAuthentication(gomock.Any(), "name")

	err := s.service().EnableUserAuthentication(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
}

// TestDisableUserAuthentication tests the happy path for DisableUserAuthentication.
func (s *serviceSuite) TestDisableUserAuthentication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DisableUserAuthentication(gomock.Any(), "name")

	err := s.service().DisableUserAuthentication(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)

}

// FuzzGetUser is a fuzz test for GetUser() that stresses the username input of
// the function to make sure that no panics occur and all input is handled
// gracefully.
func FuzzGetUser(f *testing.F) {
	for _, valid := range usertesting.ValidUsernames {
		f.Add(valid)
	}

	f.Fuzz(func(t *testing.T, username string) {
		ctrl := gomock.NewController(t)
		state := NewMockState(ctrl)
		defer ctrl.Finish()

		state.EXPECT().GetUserByName(gomock.Any(), username).Return(
			user.User{
				Name: username,
			},
			nil,
		).AnyTimes()

		usr, err := NewService(state).GetUserByName(context.Background(), username)
		if err != nil && !errors.Is(err, usererrors.UserNameNotValid) {
			t.Errorf("unexpected error %v when fuzzing GetUser with %q",
				err, username,
			)
		} else if errors.Is(err, usererrors.UserNameNotValid) {
			return
		}

		if usr.Name != username {
			t.Errorf("GetUser() user.name %q != %q", usr.Name, username)
		}
	})
}

// TestUsernameValidation exists to assert the regex that is in use by
// ValidateUserName. We want to pass it a wide range of unicode names with weird
func (s *serviceSuite) TestUserNameValidation(c *gc.C) {
	var tests []struct {
		Username   string
		ShouldPass bool
	}

	for _, valid := range usertesting.ValidUsernames {
		tests = append(tests, struct {
			Username   string
			ShouldPass bool
		}{valid, true})
	}

	for _, invalid := range usertesting.InvalidUsernames {
		tests = append(tests, struct {
			Username   string
			ShouldPass bool
		}{invalid, false})
	}

	for _, test := range tests {
		err := domainuser.ValidateUserName(test.Username)
		if test.ShouldPass {
			c.Assert(err, jc.ErrorIsNil, gc.Commentf("test username %q", test.Username))
		} else {
			c.Assert(
				err, jc.ErrorIs, usererrors.UserNameNotValid,
				gc.Commentf("test username %q", test.Username),
			)
		}
	}
}

// TestUpdateLastLogin tests the happy path for UpdateLastLogin.
func (s *serviceSuite) TestUpdateLastLogin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpdateLastLogin(gomock.Any(), "name")

	err := s.service().UpdateLastLogin(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
}

type stringerNotEmpty struct{}

func (s stringerNotEmpty) Matches(arg any) bool {
	str, ok := arg.(fmt.Stringer)
	if !ok {
		return false
	}
	return str.String() != ""
}

func (s stringerNotEmpty) String() string {
	return "matches if the input fmt.Stringer produces a non-empty string."
}
