// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/user/errors"
)

type serviceSuite struct {
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

var (
	invalidUsernames = []string{
		"😱",  // We don't support emoji's
		"+蓮", // Cannot start with a +
		"-蓮", // Cannot start with a -
		".蓮", // Cannot start with a .
		"蓮+", // Cannot end with a +
		"蓮-", // Cannot end with a -
		"蓮.", // Cannot end with a .
	}
)

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state)
}

func (s *serviceSuite) TestAddUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := "f00-Bar.ram77"
	displayName := "Display"
	password := "password"
	creator := "admin"

	s.state.EXPECT().AddUser(gomock.Any(), name, displayName, password, creator).Return(nil)

	err := s.service().AddUser(context.Background(), name, displayName, password, creator)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := "f00-Bar.ram77"
	s.state.EXPECT().RemoveUser(gomock.Any(), name).Return(nil)
	err := s.service().RemoveUser(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := "f00-Bar.ram77"
	password := user.NewUserPassword("password")
	lowercaseName := strings.ToLower(name)

	s.state.EXPECT().RemoveUserActivationKey(gomock.Any(), lowercaseName).Return(nil)
	s.state.EXPECT().SetPassword(gomock.Any(), lowercaseName, password).Return(nil)

	err := s.service().SetPassword(context.Background(), lowercaseName, password)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGenerateActivationKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := "f00-Bar.ram77"
	lowercaseName := strings.ToLower(name)

	s.state.EXPECT().GenerateUserActivationKey(gomock.Any(), lowercaseName).Return(nil)

	err := s.service().GenerateUserActivationKey(context.Background(), lowercaseName)
	c.Assert(err, jc.ErrorIsNil)
}

// TestGetUserNotFound is testing what the service does when we ask for a user
// that doesn't exist. The expected behaviour is that an error is returned that
// satisfies usererrors.NotFound.
func (s *serviceSuite) TestGetUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUser(gomock.Any(), "اقتدار").Return(user.User{}, usererrors.NotFound)
	_, err := s.service().GetUser(context.Background(), "اقتدار")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestGetUser is asserting the safe path of GetUser in that if we supply a
// happy and good username and the username exists in state we get back a valid
// user object.
func (s *serviceSuite) TestGetUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	usersState := map[string]user.User{
		"Jürgen.test": user.User{
			CreatedAt:   time.Now().Add(-time.Minute * 5),
			DisplayName: "Old mate 👍",
			Name:        "Jürgen.test",
		},
		"杨-test": user.User{
			CreatedAt:   time.Now().Add(-time.Minute * 5),
			DisplayName: "test1",
			Name:        "杨-test",
		},
	}

	for userName, user := range usersState {
		s.state.EXPECT().GetUser(gomock.Any(), userName).Return(usersState[userName], nil)
		rval, err := s.service().GetUser(context.Background(), userName)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rval, jc.DeepEquals, user)
	}
}

// TestGetUserInvalidUsername is here to assert that when we ask for a user with
// a username that is invalid we get a UsernameNotValid error. We also check
// here that the service doesn't let invalid usernames flow through to the state
// layer.
func (s *serviceSuite) TestGetUserInvalidUsername(c *gc.C) {
	for _, invalid := range invalidUsernames {
		_, err := s.service().GetUser(context.Background(), invalid)
		c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)
	}
}

// TestUsernameValidation exists to assert the regex that is in use by
// ValidateUsername. We want to pass it a wide range of unicode names with weird
func (s *serviceSuite) TestUsernameValidation(c *gc.C) {
	tests := []struct {
		Username   string
		ShouldPass bool
	}{
		{"蓮", true}, // Ren in Japanese
		{"wallyworld", true},
		{"r", true}, // username for Rob Pike, fixes lp1620444
		{"Jürgen.test", true},
		{"Günter+++test", true},
		{"王", true},      // Wang in Chinese
		{"杨-test", true}, // Yang in Chinese
		{"اقتدار", true},

		// Some Romanian usernames. Thanks Dora!!!
		{"Alinuța", true},
		{"Bulișor", true},
		{"Gheorghiță", true},
		{"Mărioara", true},
		{"Vasilică", true},

		// Some Turkish usernames, Thanks Caner!!!
		{"rüştü", true},
		{"özlem", true},
		{"yağız", true},
		{"f00-Bar.ram77", true},
	}

	for _, invalid := range invalidUsernames {
		tests = append(tests, struct {
			Username   string
			ShouldPass bool
		}{invalid, false})
	}

	for _, test := range tests {
		err := ValidateUsername(test.Username)
		if test.ShouldPass {
			c.Assert(err, jc.ErrorIsNil, gc.Commentf("test username %q", test.Username))
		} else {
			c.Assert(
				err, jc.ErrorIs, usererrors.UsernameNotValid,
				gc.Commentf("test username %q", test.Username),
			)
		}
	}
}
