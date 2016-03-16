// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type ChangePasswordCommandSuite struct {
	BaseSuite
	mockAPI        *mockChangePasswordAPI
	store          jujuclient.ClientStore
	randomPassword string
}

var _ = gc.Suite(&ChangePasswordCommandSuite{})

func (s *ChangePasswordCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.randomPassword = ""
	s.store = s.BaseSuite.store
	s.PatchValue(user.RandomPasswordNotify, func(pwd string) {
		s.randomPassword = pwd
	})
}

func (s *ChangePasswordCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	changePasswordCommand, _ := user.NewChangePasswordCommandForTest(s.mockAPI, s.store)
	return coretesting.RunCommand(c, changePasswordCommand, args...)
}

func (s *ChangePasswordCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		generate    bool
		errorString string
	}{
		{
		// no args is fine
		}, {
			args:     []string{"--generate"},
			generate: true,
		}, {
			args:     []string{"foobar"},
			user:     "foobar",
			generate: true,
		}, {
			args:     []string{"foobar", "--generate"},
			user:     "foobar",
			generate: true,
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
			c.Check(command.Generate, gc.Equals, test.generate)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *ChangePasswordCommandSuite) assertSetPassword(c *gc.C, user, pass string) {
	s.assertSetPasswordN(c, 0, user, pass)
}

func (s *ChangePasswordCommandSuite) assertSetPasswordN(c *gc.C, n int, user, pass string) {
	s.mockAPI.CheckCall(c, n+1, "SetPassword", user, pass)
}

func (s *ChangePasswordCommandSuite) assertStorePassword(c *gc.C, user, pass string) {
	details, err := s.store.AccountByName("testing", user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Password, gc.Equals, pass)
}

func (s *ChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSetPassword(c, "current-user@local", "sekrit")
	expected := `
password: 
type password again: 
`[1:]
	c.Assert(coretesting.Stdout(context), gc.Equals, expected)
	c.Assert(coretesting.Stderr(context), gc.Equals, "Your password has been updated.\n")
}

func (s *ChangePasswordCommandSuite) TestChangePasswordGenerate(c *gc.C) {
	context, err := s.run(c, "--generate")
	c.Assert(err, jc.ErrorIsNil)
	s.assertSetPassword(c, "current-user@local", s.randomPassword)
	c.Assert(coretesting.Stderr(context), gc.Equals, "Your password has been updated.\n")
}

func (s *ChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.SetErrors(nil, errors.New("failed to do something"))
	_, err := s.run(c, "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	s.assertSetPassword(c, "current-user@local", s.randomPassword)
}

// We create a macaroon, but fail to write it to accounts.yaml.
// We should not call SetPassword subsequently.
func (s *ChangePasswordCommandSuite) TestNoSetPasswordAfterFailedWrite(c *gc.C) {
	store := jujuclienttesting.NewStubStore()
	store.CurrentAccountFunc = func(string) (string, error) {
		return "account-name", nil
	}
	store.AccountByNameFunc = func(string, string) (*jujuclient.AccountDetails, error) {
		return &jujuclient.AccountDetails{"user", "old-password", ""}, nil
	}
	store.ControllerByNameFunc = func(string) (*jujuclient.ControllerDetails, error) {
		return &jujuclient.ControllerDetails{}, nil
	}
	s.store = store
	store.SetErrors(errors.New("failed to write"))

	_, err := s.run(c, "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to update client credentials: failed to write")
	s.mockAPI.CheckCallNames(c, "CreateLocalLoginMacaroon") // no SetPassword
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPassword(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	_, err := s.run(c, "other")
	c.Assert(err, jc.ErrorIsNil)
	s.assertSetPassword(c, "other@local", s.randomPassword)
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
