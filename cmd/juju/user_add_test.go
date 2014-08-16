// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	goyaml "gopkg.in/yaml.v1"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "user add" command
type UserAddCommandSuite struct {
	testing.FakeJujuHomeSuite
	mockAPI *mockAddUserAPI
}

var _ = gc.Suite(&UserAddCommandSuite{})

func (s *UserAddCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.mockAPI = &mockAddUserAPI{}
	s.PatchValue(&getAddUserAPI, func(c *UserAddCommand) (addUserAPI, error) {
		return s.mockAPI, nil
	})
}

func newUserAddCommand() cmd.Command {
	return envcmd.Wrap(&UserAddCommand{})
}

func (s *UserAddCommandSuite) TestAddUserJustUsername(c *gc.C) {
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "")
	// Password is generated
	c.Assert(s.mockAPI.password, gc.Not(gc.Equals), "")
	expected := fmt.Sprintf(`user "foobar" added with password %q`, s.mockAPI.password)
	c.Assert(testing.Stdout(context), gc.Equals, expected+"\n")
}

func (s *UserAddCommandSuite) TestAddUserUsernameAndDisplayname(c *gc.C) {
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "Foo Bar")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "Foo Bar")
	// Password is generated
	c.Assert(s.mockAPI.password, gc.Not(gc.Equals), "")
	expected := fmt.Sprintf(`user "Foo Bar (foobar)" added with password %q`, s.mockAPI.password)
	c.Assert(testing.Stdout(context), gc.Equals, expected+"\n")
}

func (s *UserAddCommandSuite) TestAddUserUsernameAndDisplaynameWithPassword(c *gc.C) {
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "Foo Bar", "--password", "password")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "Foo Bar")
	c.Assert(s.mockAPI.password, gc.Equals, "password")
	expected := `user "Foo Bar (foobar)" added with password "password"`
	c.Assert(testing.Stdout(context), gc.Equals, expected+"\n")
}

func (s *UserAddCommandSuite) TestAddUserErrorResponse(c *gc.C) {
	s.mockAPI.failMessage = "failed to create user, chaos ensues"
	context, err := testing.RunCommand(c, newUserAddCommand(), "foobar")
	c.Assert(err, gc.ErrorMatches, "failed to create user, chaos ensues")
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "")
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

func (s *UserAddCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		displayname string
		password    string
		outPath     string
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
			args:     []string{"foobar", "--password", "password"},
			user:     "foobar",
			password: "password",
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
		addUserCmd := &UserAddCommand{}
		err := testing.InitCommand(addUserCmd, test.args)
		if test.errorString == "" {
			c.Check(addUserCmd.User, gc.Equals, test.user)
			c.Check(addUserCmd.DisplayName, gc.Equals, test.displayname)
			c.Check(addUserCmd.Password, gc.Equals, test.password)
			c.Check(addUserCmd.OutPath, gc.Equals, test.outPath)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func fakeBootstrapEnvironment(c *gc.C, envName string) {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	envInfo := store.CreateInfo(envName)
	envInfo.SetBootstrapConfig(map[string]interface{}{"random": "extra data"})
	envInfo.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: []string{"localhost:12345"},
		CACert:    testing.CACert,
	})
	envInfo.SetAPICredentials(configstore.APICredentials{
		User:     "admin",
		Password: "password",
	})
	err = envInfo.Write()
	c.Assert(err, gc.IsNil)
}

func (s *UserAddCommandSuite) TestJenvOutput(c *gc.C) {
	fakeBootstrapEnvironment(c, "erewhemos")
	outputName := filepath.Join(c.MkDir(), "output")
	ctx, err := testing.RunCommand(c, newUserAddCommand(),
		"foobar", "--password", "password", "--output", outputName)
	c.Assert(err, gc.IsNil)

	expected := fmt.Sprintf(`user "foobar" added with password %q`, s.mockAPI.password)
	expected = fmt.Sprintf("%s\nenvironment file written to %s.jenv\n", expected, outputName)
	c.Assert(testing.Stdout(ctx), gc.Equals, expected)

	raw, err := ioutil.ReadFile(outputName + ".jenv")
	c.Assert(err, gc.IsNil)
	d := map[string]interface{}{}
	err = goyaml.Unmarshal(raw, &d)
	c.Assert(err, gc.IsNil)
	c.Assert(d["user"], gc.Equals, "foobar")
	c.Assert(d["password"], gc.Equals, "password")
	c.Assert(d["state-servers"], gc.DeepEquals, []interface{}{"localhost:12345"})
	c.Assert(d["ca-cert"], gc.DeepEquals, testing.CACert)
	_, found := d["bootstrap-config"]
	c.Assert(found, jc.IsFalse)
}

type mockAddUserAPI struct {
	failMessage string
	username    string
	displayname string
	password    string
}

func (m *mockAddUserAPI) AddUser(username, displayname, password string) error {
	m.username = username
	m.displayname = displayname
	m.password = password
	if m.failMessage == "" {
		return nil
	}
	return errors.New(m.failMessage)
}

func (*mockAddUserAPI) Close() error {
	return nil
}
