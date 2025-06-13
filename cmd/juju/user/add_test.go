// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"context"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "add-user" command
type UserAddCommandSuite struct {
	BaseSuite
	mockAPI *mockAddUserAPI
}

func TestUserAddCommandSuite(t *stdtesting.T) {
	tc.Run(t, &UserAddCommandSuite{})
}

func (s *UserAddCommandSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockAddUserAPI{}
	s.mockAPI.secretKey = []byte(strings.Repeat("X", 32))
}

func (s *UserAddCommandSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	addCommand, _ := user.NewAddCommandForTest(s.mockAPI, s.store, &mockModelAPI{})
	return cmdtesting.RunCommand(c, addCommand, args...)
}

func (s *UserAddCommandSuite) TestInit(c *tc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		displayname string
		models      string
		acl         string
		outPath     string
		errorString string
	}{{
		errorString: "no username supplied",
	}, {
		args: []string{"foobar"},
		user: "foobar",
	}, {
		args:        []string{"foobar", "Foo Bar"},
		user:        "foobar",
		displayname: "Foo Bar",
	}, {
		args:        []string{"foobar", "Foo Bar", "extra"},
		errorString: `unrecognized args: \["extra"\]`,
	}, {
		args: []string{"foobar"},
		user: "foobar",
	}, {
		args: []string{"foobar"},
		user: "foobar",
	}} {
		c.Logf("test %d (%q)", i, test.args)
		wrappedCommand, command := user.NewAddCommandForTest(s.mockAPI, s.store, &mockModelAPI{})
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, tc.ErrorIsNil)
			c.Check(command.User, tc.Equals, test.user)
			c.Check(command.DisplayName, tc.Equals, test.displayname)
		} else {
			c.Check(err, tc.ErrorMatches, test.errorString)
		}
	}
}

func (s *UserAddCommandSuite) TestAddUserWithUsername(c *tc.C) {
	context, err := s.run(c, "foobar")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.username, tc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, tc.Equals, "")
	expected := `
User "foobar" added
Please send this command to foobar:
    juju register MEYTBmZvb2JhcjAPEw0wLjEuMi4zOjEyMzQ1BCBYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWBMHdGVzdGluZxMA

"foobar" has not been granted access to any models. You can use "juju grant" to grant access.
`[1:]
	c.Assert(cmdtesting.Stdout(context), tc.Equals, expected)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
}

func (s *UserAddCommandSuite) TestAddUserWithUsernameAndDisplayname(c *tc.C) {
	context, err := s.run(c, "foobar", "Foo Bar")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.username, tc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, tc.Equals, "Foo Bar")
	expected := `
User "Foo Bar (foobar)" added
Please send this command to foobar:
    juju register MEYTBmZvb2JhcjAPEw0wLjEuMi4zOjEyMzQ1BCBYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWBMHdGVzdGluZxMA

"Foo Bar (foobar)" has not been granted access to any models. You can use "juju grant" to grant access.
`[1:]
	c.Assert(cmdtesting.Stdout(context), tc.Equals, expected)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
}

func (s *UserAddCommandSuite) TestUserRegistrationString(c *tc.C) {
	// Ensure that the user registration string only contains alphanumerics.
	for i := 0; i < 3; i++ {
		s.mockAPI.secretKey = []byte(strings.Repeat("X", 32+i))
		context, err := s.run(c, "foobar", "Foo Bar")
		c.Assert(err, tc.ErrorIsNil)
		lines := strings.Split(cmdtesting.Stdout(context), "\n")
		c.Assert(lines, tc.HasLen, 6)
		c.Assert(lines[2], tc.Matches, `^\s+juju register [A-Za-z0-9]+$`)
	}
}

type mockModelAPI struct{}

func (m *mockModelAPI) ListModels(ctx context.Context, user string) ([]base.UserModel, error) {
	return []base.UserModel{{Name: "model", UUID: "modeluuid", Qualifier: "prod"}}, nil
}

func (m *mockModelAPI) Close() error {
	return nil
}

func (s *UserAddCommandSuite) TestBlockAddUser(c *tc.C) {
	// Block operation
	s.mockAPI.blocked = true
	_, err := s.run(c, "foobar", "Foo Bar")
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *UserAddCommandSuite) TestAddUserErrorResponse(c *tc.C) {
	s.mockAPI.failMessage = "failed to create user, chaos ensues"
	_, err := s.run(c, "foobar")
	c.Assert(err, tc.ErrorMatches, s.mockAPI.failMessage)
}

func (s *UserAddCommandSuite) TestAddUserUnauthorizedMentionsJujuGrant(c *tc.C) {
	s.mockAPI.addError = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := s.run(c, "foobar")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, tc.Matches, `.*juju grant.*`)
}

type mockAddUserAPI struct {
	addError    error
	failMessage string
	blocked     bool
	secretKey   []byte

	username    string
	displayname string
	password    string
}

func (m *mockAddUserAPI) AddUser(ctx context.Context, username, displayname, password string) (names.UserTag, []byte, error) {
	if m.blocked {
		return names.UserTag{}, nil, apiservererrors.OperationBlockedError("the operation has been blocked")
	}
	if m.addError != nil {
		return names.UserTag{}, nil, m.addError
	}
	m.username = username
	m.displayname = displayname
	m.password = password
	if m.failMessage != "" {
		return names.UserTag{}, nil, errors.New(m.failMessage)
	}
	return names.NewLocalUserTag(username), m.secretKey, nil
}

func (*mockAddUserAPI) Close() error {
	return nil
}
