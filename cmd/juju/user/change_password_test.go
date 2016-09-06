// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type ChangePasswordCommandSuite struct {
	BaseSuite
	mockAPI *mockChangePasswordAPI
	store   jujuclient.ClientStore
}

var _ = gc.Suite(&ChangePasswordCommandSuite{})

func (s *ChangePasswordCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.store = s.BaseSuite.store
}

func (s *ChangePasswordCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	changePasswordCommand, _ := user.NewChangePasswordCommandForTest(s.mockAPI, s.store)
	ctx := coretesting.Context(c)
	ctx.Stdin = strings.NewReader("sekrit\nsekrit\n")
	err := coretesting.InitCommand(changePasswordCommand, args)
	if err != nil {
		return ctx, err
	}
	return ctx, changePasswordCommand.Run(ctx)
}

func (s *ChangePasswordCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		errorString string
	}{
		{
		// no args is fine
		}, {
			args: []string{"foobar"},
			user: "foobar",
		}, {
			args:        []string{"--foobar"},
			errorString: "flag provided but not defined: --foobar",
		}, {
			args:        []string{"foobar", "extra"},
			errorString: `unrecognized args: \["extra"\]`,
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, command := user.NewChangePasswordCommandForTest(nil, s.store)
		err := coretesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(command.User, gc.Equals, test.user)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *ChangePasswordCommandSuite) assertAPICalls(c *gc.C, user, pass string) {
	var offset int
	if user == "current-user@local" {
		s.mockAPI.CheckCall(c, 0, "CreateLocalLoginMacaroon", names.NewUserTag(user))
		offset += 1
	}
	s.mockAPI.CheckCall(c, offset, "SetPassword", user, pass)
}

func (s *ChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAPICalls(c, "current-user@local", "sekrit")
	c.Assert(coretesting.Stdout(context), gc.Equals, "")
	c.Assert(coretesting.Stderr(context), gc.Equals, `
password: 
type password again: 
Your password has been updated.
`[1:])
}

func (s *ChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.SetErrors(nil, errors.New("failed to do something"))
	_, err := s.run(c)
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	s.assertAPICalls(c, "current-user@local", "sekrit")
}

// We create a macaroon, but fail to write it to accounts.yaml.
// We should not call SetPassword subsequently.
func (s *ChangePasswordCommandSuite) TestNoSetPasswordAfterFailedWrite(c *gc.C) {
	store := jujuclienttesting.NewStubStore()
	store.AccountDetailsFunc = func(string) (*jujuclient.AccountDetails, error) {
		return &jujuclient.AccountDetails{"user", "old-password", "", ""}, nil
	}
	store.ControllerByNameFunc = func(string) (*jujuclient.ControllerDetails, error) {
		return &jujuclient.ControllerDetails{}, nil
	}
	s.store = store
	store.SetErrors(nil, errors.New("failed to write"))

	_, err := s.run(c)
	c.Assert(err, gc.ErrorMatches, "failed to update client credentials: failed to write")
	s.mockAPI.CheckCallNames(c, "CreateLocalLoginMacaroon") // no SetPassword
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPassword(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	_, err := s.run(c, "other")
	c.Assert(err, jc.ErrorIsNil)
	s.assertAPICalls(c, "other@local", "sekrit")
}

type mockChangePasswordAPI struct {
	testing.Stub
}

func (m *mockChangePasswordAPI) CreateLocalLoginMacaroon(tag names.UserTag) (*macaroon.Macaroon, error) {
	m.MethodCall(m, "CreateLocalLoginMacaroon", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return fakeLocalLoginMacaroon(tag), nil
}

func (m *mockChangePasswordAPI) SetPassword(username, password string) error {
	m.MethodCall(m, "SetPassword", username, password)
	return m.NextErr()
}

func (*mockChangePasswordAPI) Close() error {
	return nil
}

func fakeLocalLoginMacaroon(tag names.UserTag) *macaroon.Macaroon {
	mac, err := macaroon.New([]byte("abcdefghijklmnopqrstuvwx"), tag.Canonical(), "juju")
	if err != nil {
		panic(err)
	}
	return mac
}
