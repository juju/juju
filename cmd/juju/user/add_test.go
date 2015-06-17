// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "user add" command
type UserAddCommandSuite struct {
	BaseSuite
	mockAPI        *mockAddUserAPI
	randomPassword string
	serverFilename string
}

var _ = gc.Suite(&UserAddCommandSuite{})

func (s *UserAddCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockAddUserAPI{}
	s.randomPassword = ""
	s.serverFilename = ""
	s.PatchValue(user.RandomPasswordNotify, func(pwd string) {
		s.randomPassword = pwd
	})
	s.PatchValue(user.ServerFileNotify, func(filename string) {
		s.serverFilename = filename
	})
}

func (s *UserAddCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	addCommand := envcmd.WrapSystem(user.NewAddCommand(s.mockAPI))
	return testing.RunCommand(c, addCommand, args...)
}

func (s *UserAddCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		displayname string
		outPath     string
		errorString string
	}{
		{
			errorString: "no username supplied",
		}, {
			args:    []string{"foobar"},
			user:    "foobar",
			outPath: "foobar.server",
		}, {
			args:        []string{"foobar", "Foo Bar"},
			user:        "foobar",
			displayname: "Foo Bar",
			outPath:     "foobar.server",
		}, {
			args:        []string{"foobar", "Foo Bar", "extra"},
			errorString: `unrecognized args: \["extra"\]`,
		}, {
			args:    []string{"foobar", "--output", "somefile"},
			user:    "foobar",
			outPath: "somefile",
		}, {
			args:    []string{"foobar", "-o", "somefile"},
			user:    "foobar",
			outPath: "somefile",
		},
	} {
		c.Logf("test %d", i)
		addUserCmd := &user.AddCommand{}
		err := testing.InitCommand(addUserCmd, test.args)
		if test.errorString == "" {
			c.Check(addUserCmd.User, gc.Equals, test.user)
			c.Check(addUserCmd.DisplayName, gc.Equals, test.displayname)
			c.Check(addUserCmd.OutPath, gc.Equals, test.outPath)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *UserAddCommandSuite) TestRandomPassword(c *gc.C) {
	_, err := s.run(c, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.randomPassword, gc.HasLen, 24)
}

func (s *UserAddCommandSuite) TestUsername(c *gc.C) {
	context, err := s.run(c, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "")
	expected := `
user "foobar" added
server file written to .*foobar.server
`[1:]
	c.Assert(testing.Stderr(context), gc.Matches, expected)
	s.assertServerFileMatches(c, s.serverFilename, "foobar", s.randomPassword)
}

func (s *UserAddCommandSuite) TestUsernameAndDisplayname(c *gc.C) {
	context, err := s.run(c, "foobar", "Foo Bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "Foo Bar")
	expected := `user "Foo Bar (foobar)" added`
	c.Assert(testing.Stderr(context), jc.Contains, expected)
	s.assertServerFileMatches(c, s.serverFilename, "foobar", s.randomPassword)
}

func (s *UserAddCommandSuite) TestBlockAddUser(c *gc.C) {
	// Block operation
	s.mockAPI.blocked = true
	_, err := s.run(c, "foobar", "Foo Bar")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock changes.*")
}

func (s *UserAddCommandSuite) TestAddUserErrorResponse(c *gc.C) {
	s.mockAPI.failMessage = "failed to create user, chaos ensues"
	_, err := s.run(c, "foobar")
	c.Assert(err, gc.ErrorMatches, s.mockAPI.failMessage)
}

type mockAddUserAPI struct {
	failMessage string
	username    string
	displayname string
	password    string

	shareFailMsg string
	sharedUsers  []names.UserTag
	blocked      bool
}

func (m *mockAddUserAPI) AddUser(username, displayname, password string) (names.UserTag, error) {
	if m.blocked {
		return names.UserTag{}, common.ErrOperationBlocked("The operation has been blocked.")
	}

	m.username = username
	m.displayname = displayname
	m.password = password
	if m.failMessage == "" {
		return names.NewLocalUserTag(username), nil
	}
	return names.UserTag{}, errors.New(m.failMessage)
}

func (*mockAddUserAPI) Close() error {
	return nil
}
