// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	jujutesting "launchpad.net/juju-core/juju/testing"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere
// This suite provides basic tests for the "user add" command
type UserAddCommandSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&UserAddCommandSuite{})

func newUserAddCommand() cmd.Command {
	return envcmd.Wrap(&UserAddCommand{})
}

func (s *UserAddCommandSuite) TestAddUser(c *gc.C) {

	_, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "password")
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, newUserAddCommand(), "foobar", "newpassword")
	c.Assert(err, gc.ErrorMatches, "Failed to create user: user already exists")
}

func (s *UserAddCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "password", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *UserAddCommandSuite) TestNotEnoughArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand())
	c.Assert(err, gc.ErrorMatches, `no username supplied`)
}

func (s *UserAddCommandSuite) TestJenvYamlFileOutput(c *gc.C) {
	expected := map[string]interface{}{
		"user":          "foobar",
		"password":      "password",
		"state-servers": []interface{}{},
		"ca-cert":       ""}
	tempFile, err := ioutil.TempFile("", "adduser-test")
	tempFile.Close()
	c.Assert(err, gc.IsNil)
	_, err = testing.RunCommand(c, newUserAddCommand(), "foobar", "password", "-o", tempFile.Name())
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadFile(tempFile.Name())
	result := map[string]interface{}{}
	err = goyaml.Unmarshal(data, &result)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *UserAddCommandSuite) TestJenvYamlOutput(c *gc.C) {
	expected := map[string]interface{}{
		"user":          "foobar",
		"password":      "password",
		"state-servers": []interface{}{},
		"ca-cert":       ""}
	ctx, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "password")
	c.Assert(err, gc.IsNil)
	stdout := ctx.Stdout.(*bytes.Buffer).Bytes()
	result := map[string]interface{}{}
	err = goyaml.Unmarshal(stdout, &result)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *UserAddCommandSuite) TestJenvJsonOutput(c *gc.C) {
	expected := `{"User":"foobar","Password":"password","state-servers":null,"ca-cert":""}
`
	tempFile, err := ioutil.TempFile("", "adduser-test")
	tempFile.Close()
	c.Assert(err, gc.IsNil)
	_, err = testing.RunCommand(c, newUserAddCommand(), "foobar", "password", "-o", tempFile.Name(), "--format", "json")
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadFile(tempFile.Name())
	c.Assert(string(data), gc.DeepEquals, expected)
}

func (s *UserAddCommandSuite) TestJenvJsonFileOutput(c *gc.C) {
	expected := `{"User":"foobar","Password":"password","state-servers":null,"ca-cert":""}
`
	ctx, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "password", "--format", "json")
	c.Assert(err, gc.IsNil)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, gc.DeepEquals, expected)
}

func (s *UserAddCommandSuite) TestGeneratePassword(c *gc.C) {
	ctx, err := testing.RunCommand(c, newUserAddCommand(), "foobar")
	c.Assert(err, gc.IsNil)
	stdout := ctx.Stdout.(*bytes.Buffer).Bytes()
	var d map[string]interface{}
	err = goyaml.Unmarshal(stdout, &d)
	c.Assert(err, gc.IsNil)
	c.Assert(d["user"], gc.DeepEquals, "foobar")
}
