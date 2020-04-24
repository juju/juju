// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "add-user" command
type UserAddCommandSuite struct {
	BaseSuite
	mockAPI *mockAddUserAPI
}

var _ = gc.Suite(&UserAddCommandSuite{})

func (s *UserAddCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockAddUserAPI{}
	s.mockAPI.secretKey = []byte(strings.Repeat("X", 32))
}

func (s *UserAddCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	addCommand, _ := user.NewAddCommandForTest(s.mockAPI, s.store, &mockModelAPI{})
	return cmdtesting.RunCommand(c, addCommand, args...)
}

func (s *UserAddCommandSuite) TestInit(c *gc.C) {
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
			c.Check(err, jc.ErrorIsNil)
			c.Check(command.User, gc.Equals, test.user)
			c.Check(command.DisplayName, gc.Equals, test.displayname)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *UserAddCommandSuite) TestAddUserWithUsername(c *gc.C) {
	context, err := s.run(c, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "")
	expected := `
User "foobar" added
Please send this command to foobar:
    juju register MEQTBmZvb2JhcjAPEw0wLjEuMi4zOjEyMzQ1BCBYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWBMHdGVzdGluZwAA

"foobar" has not been granted access to any models. You can use "juju grant" to grant access.
`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expected)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

func (s *UserAddCommandSuite) TestAddUserWithUsernameAndDisplayname(c *gc.C) {
	context, err := s.run(c, "foobar", "Foo Bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "Foo Bar")
	expected := `
User "Foo Bar (foobar)" added
Please send this command to foobar:
    juju register MEQTBmZvb2JhcjAPEw0wLjEuMi4zOjEyMzQ1BCBYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWBMHdGVzdGluZwAA

"Foo Bar (foobar)" has not been granted access to any models. You can use "juju grant" to grant access.
`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expected)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

func (s *UserAddCommandSuite) TestUserRegistrationString(c *gc.C) {
	// Ensure that the user registration string only contains alphanumerics.
	for i := 0; i < 3; i++ {
		s.mockAPI.secretKey = []byte(strings.Repeat("X", 32+i))
		context, err := s.run(c, "foobar", "Foo Bar")
		c.Assert(err, jc.ErrorIsNil)
		lines := strings.Split(cmdtesting.Stdout(context), "\n")
		c.Assert(lines, gc.HasLen, 6)
		c.Assert(lines[2], gc.Matches, `^\s+juju register [A-Za-z0-9]+$`)
	}
}

type mockModelAPI struct{}

func (m *mockModelAPI) ListModels(user string) ([]base.UserModel, error) {
	return []base.UserModel{{Name: "model", UUID: "modeluuid", Owner: "current-user"}}, nil
}

func (m *mockModelAPI) Close() error {
	return nil
}

func (s *UserAddCommandSuite) TestBlockAddUser(c *gc.C) {
	// Block operation
	s.mockAPI.blocked = true
	_, err := s.run(c, "foobar", "Foo Bar")
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *UserAddCommandSuite) TestAddUserErrorResponse(c *gc.C) {
	s.mockAPI.failMessage = "failed to create user, chaos ensues"
	_, err := s.run(c, "foobar")
	c.Assert(err, gc.ErrorMatches, s.mockAPI.failMessage)
}

func (s *UserAddCommandSuite) TestAddUserUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.mockAPI.addError = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := s.run(c, "foobar")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, gc.Matches, `.*juju grant.*`)
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

func (m *mockAddUserAPI) AddUser(username, displayname, password string) (names.UserTag, []byte, error) {
	if m.blocked {
		return names.UserTag{}, nil, common.OperationBlockedError("the operation has been blocked")
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
