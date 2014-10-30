// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "user add" command
type UserAddCommandSuite struct {
	BaseSuite
	mockAPI *mockAddUserAPI
}

var _ = gc.Suite(&UserAddCommandSuite{})

func (s *UserAddCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockAddUserAPI{}
	s.PatchValue(user.GetAddUserAPI, func(c *user.AddCommand) (user.AddUserAPI, error) {
		return s.mockAPI, nil
	})
	s.PatchValue(user.GetShareEnvAPI, func(c *user.AddCommand) (user.ShareEnvironmentAPI, error) {
		return s.mockAPI, nil
	})
}

func newUserAddCommand() cmd.Command {
	return envcmd.Wrap(&user.AddCommand{})
}

func (s *UserAddCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		displayname string
		outPath     string
		generate    bool
		errorString string
	}{
		{
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
			args:     []string{"foobar", "--generate"},
			user:     "foobar",
			generate: true,
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
			c.Check(addUserCmd.Generate, gc.Equals, test.generate)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func assertJENVContents(c *gc.C, filename, username, password string) {
	raw, err := ioutil.ReadFile(filename)
	c.Assert(err, gc.IsNil)
	details := map[string]interface{}{}
	err = goyaml.Unmarshal(raw, &details)
	c.Assert(err, gc.IsNil)
	c.Assert(details["user"], gc.Equals, username)
	c.Assert(details["password"], gc.Equals, password)
	c.Assert(details["state-servers"], gc.DeepEquals, []interface{}{"localhost:12345"})
	c.Assert(details["ca-cert"], gc.DeepEquals, testing.CACert)
	c.Assert(details["environ-uuid"], gc.Equals, "env-uuid")
	_, found := details["bootstrap-config"]
	c.Assert(found, jc.IsFalse)
}

func (s *UserAddCommandSuite) AssertJENVContents(c *gc.C, filename string) {
	assertJENVContents(c, filename, s.mockAPI.username, s.mockAPI.password)
}

func (s *UserAddCommandSuite) TestAddUserJustUsername(c *gc.C) {
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "")
	c.Assert(s.mockAPI.password, gc.Equals, "sekrit")
	expected := `
password:
type password again:
user "foobar" added
environment file written to .*foobar.jenv
`[1:]
	c.Assert(testing.Stdout(context), gc.Matches, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "To generate a random strong password, use the --generate flag.\n")
	s.AssertJENVContents(c, context.AbsPath("foobar.jenv"))
}

func (s *UserAddCommandSuite) TestAddUserUsernameAndDisplayname(c *gc.C) {
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "Foo Bar")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "Foo Bar")
	expected := `user "Foo Bar (foobar)" added`
	c.Assert(testing.Stdout(context), jc.Contains, expected)
	s.AssertJENVContents(c, context.AbsPath("foobar.jenv"))
}

func (s *UserAddCommandSuite) TestGeneratePassword(c *gc.C) {
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "--generate")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.password, gc.Not(gc.Equals), "sekrit")
	c.Assert(s.mockAPI.password, gc.HasLen, 24)
	expected := `
user "foobar" added
environment file written to .*foobar.jenv
`[1:]
	c.Assert(testing.Stdout(context), gc.Matches, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "")
	s.AssertJENVContents(c, context.AbsPath("foobar.jenv"))
}

func (s *UserAddCommandSuite) TestAddUserErrorResponse(c *gc.C) {
	s.mockAPI.failMessage = "failed to create user, chaos ensues"
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to create user, chaos ensues")
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "")
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

func (s *UserAddCommandSuite) TestJenvOutput(c *gc.C) {
	outputName := filepath.Join(c.MkDir(), "output")
	context, err := testing.RunCommand(c, newUserAddCommand(),
		"foobar", "--output", outputName)
	c.Assert(err, gc.IsNil)
	s.AssertJENVContents(c, context.AbsPath(outputName+".jenv"))
}

func (s *UserAddCommandSuite) TestJenvOutputWithSuffix(c *gc.C) {
	outputName := filepath.Join(c.MkDir(), "output.jenv")
	context, err := testing.RunCommand(c, newUserAddCommand(),
		"foobar", "--output", outputName)
	c.Assert(err, gc.IsNil)
	s.AssertJENVContents(c, context.AbsPath(outputName))
}

type mockAddUserAPI struct {
	failMessage string
	username    string
	displayname string
	password    string

	shareFailMsg string
	sharedUsers  []names.UserTag
}

func (m *mockAddUserAPI) AddUser(username, displayname, password string) (names.UserTag, error) {
	m.username = username
	m.displayname = displayname
	m.password = password
	if m.failMessage == "" {
		return names.NewLocalUserTag(username), nil
	}
	return names.UserTag{}, errors.New(m.failMessage)
}

func (m *mockAddUserAPI) ShareEnvironment(users []names.UserTag) error {
	if m.shareFailMsg != "" {
		return errors.New(m.shareFailMsg)
	}
	m.sharedUsers = users
	return nil
}

func (*mockAddUserAPI) Close() error {
	return nil
}
