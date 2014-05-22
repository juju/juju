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

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "user add" command
type UserAddCommandSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&UserAddCommandSuite{})

func newUserAddCommand() cmd.Command {
	return envcmd.Wrap(&UserAddCommand{})
}

func (s *UserAddCommandSuite) TestUserAdd(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand(), []string{"foobar"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, newUserAddCommand(), []string{"foobar"})
	c.Assert(err, gc.ErrorMatches, "Failed to create user: user already exists")
}

func (s *UserAddCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand(), []string{"foobar", "whoops"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *UserAddCommandSuite) TestNotEnoughArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand(), []string{})
	c.Assert(err, gc.ErrorMatches, `no username supplied`)
}

func (s *UserAddCommandSuite) TestGeneratePassword(c *gc.C) {
	ctx, err := testing.RunCommand(c, newUserAddCommand(), []string{"foobar"})

	c.Assert(err, gc.IsNil)
	d := decodeYamlFromStdout(c, ctx)
	c.Assert(d["user"], gc.Equals, "foobar")
	// Let's not try to assume too much about the password generation
	// algorithm other than there will be at least 10 characters.
	c.Assert(d["password"], gc.Matches, "..........+")
}

func (s *UserAddCommandSuite) TestUserSpecifiedPassword(c *gc.C) {
	ctx, err := testing.RunCommand(c, newUserAddCommand(), []string{"foobar", "--password", "frogdog"})
	c.Assert(err, gc.IsNil)

	d := decodeYamlFromStdout(c, ctx)
	c.Assert(d["user"], gc.DeepEquals, "foobar")
	c.Assert(d["password"], gc.DeepEquals, "frogdog")
}

func (s *UserAddCommandSuite) TestJenvYamlOutput(c *gc.C) {
	ctx, err := testing.RunCommand(c, newUserAddCommand(), []string{"foobar", "--password=password"})
	c.Assert(err, gc.IsNil)
	d := decodeYamlFromStdout(c, ctx)
	c.Assert(d, gc.DeepEquals, map[string]interface{}{
		"user":          "foobar",
		"password":      "password",
		"state-servers": []interface{}{},
		"ca-cert":       "",
	})
}

func (s *UserAddCommandSuite) TestJenvYamlFileOutput(c *gc.C) {
	tempFile, err := ioutil.TempFile("", "useradd-test")
	tempFile.Close()
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, newUserAddCommand(),
		[]string{"foobar", "--password", "password", "-o", tempFile.Name()})
	c.Assert(err, gc.IsNil)

	raw, err := ioutil.ReadFile(tempFile.Name())
	c.Assert(err, gc.IsNil)
	d := decodeYaml(c, raw)
	c.Assert(d, gc.DeepEquals, map[string]interface{}{
		"user":          "foobar",
		"password":      "password",
		"state-servers": []interface{}{},
		"ca-cert":       "",
	})
}

func (s *UserAddCommandSuite) TestJenvJsonOutput(c *gc.C) {
	ctx, err := testing.RunCommand(c, newUserAddCommand(),
		[]string{"foobar", "--password", "password", "--format", "json"})
	c.Assert(err, gc.IsNil)

	c.Assert(testing.Stdout(ctx), gc.Equals,
		`{"User":"foobar","Password":"password","state-servers":null,"ca-cert":""}
`)
}

func (s *UserAddCommandSuite) TestJenvJsonFileOutput(c *gc.C) {
	tempFile, err := ioutil.TempFile("", "useradd-test")
	c.Assert(err, gc.IsNil)
	tempFile.Close()

	_, err = testing.RunCommand(c, newUserAddCommand(),
		[]string{"foobar", "--password=password", "-o", tempFile.Name(), "--format", "json"})
	c.Assert(err, gc.IsNil)

	data, err := ioutil.ReadFile(tempFile.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.DeepEquals,
		`{"User":"foobar","Password":"password","state-servers":null,"ca-cert":""}
`)
}

func decodeYamlFromStdout(c *gc.C, ctx *cmd.Context) map[string]interface{} {
	return decodeYaml(c, ctx.Stdout.(*bytes.Buffer).Bytes())
}

func decodeYaml(c *gc.C, raw []byte) map[string]interface{} {
	result := map[string]interface{}{}
	err := goyaml.Unmarshal(raw, &result)
	c.Assert(err, gc.IsNil)
	return result
}
