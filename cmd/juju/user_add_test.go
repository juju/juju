// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "user add" command
type UserAddCommandSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&UserAddCommandSuite{})

func newUserAddCommand() cmd.Command {
	return envcmd.Wrap(&UserAddCommand{})
}

func (s *UserAddCommandSuite) TestUserAdd(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand(), "foobar")
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, newUserAddCommand(), "foobar")
	c.Assert(err, gc.ErrorMatches, "Failed to create user: user already exists")
}

func (s *UserAddCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *UserAddCommandSuite) TestNotEnoughArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserAddCommand())
	c.Assert(err, gc.ErrorMatches, `no username supplied`)
}

func (s *UserAddCommandSuite) TestGeneratePassword(c *gc.C) {
	ctx, err := testing.RunCommand(c, newUserAddCommand(), "foobar")

	c.Assert(err, gc.IsNil)
	user, password, filename := parseUserAddStdout(c, ctx)
	c.Assert(user, gc.Equals, "foobar")
	// Let's not try to assume too much about the password generation
	// algorithm other than there will be at least 10 characters.
	c.Assert(password, gc.Matches, "..........+")
	c.Assert(filename, gc.Equals, "")
}

func (s *UserAddCommandSuite) TestUserSpecifiedPassword(c *gc.C) {
	ctx, err := testing.RunCommand(c, newUserAddCommand(), "foobar", "--password", "frogdog")
	c.Assert(err, gc.IsNil)

	user, password, filename := parseUserAddStdout(c, ctx)
	c.Assert(user, gc.Equals, "foobar")
	c.Assert(password, gc.Equals, "frogdog")
	c.Assert(filename, gc.Equals, "")
}

func (s *UserAddCommandSuite) TestJenvOutput(c *gc.C) {
	outputName := filepath.Join(c.MkDir(), "output")
	ctx, err := testing.RunCommand(c, newUserAddCommand(),
		"foobar", "--password", "password", "--output", outputName)
	c.Assert(err, gc.IsNil)

	user, password, filename := parseUserAddStdout(c, ctx)
	c.Assert(user, gc.Equals, "foobar")
	c.Assert(password, gc.Equals, "password")
	c.Assert(filename, gc.Equals, outputName+".jenv")

	raw, err := ioutil.ReadFile(filename)
	c.Assert(err, gc.IsNil)
	d := map[string]interface{}{}
	err = goyaml.Unmarshal(raw, &d)
	c.Assert(err, gc.IsNil)
	c.Assert(d["user"], gc.Equals, "foobar")
	c.Assert(d["password"], gc.Equals, "password")
	_, found := d["state-servers"]
	c.Assert(found, gc.Equals, true)
	_, found = d["ca-cert"]
	c.Assert(found, gc.Equals, true)
}

// parseUserAddStdout parses the output from the "juju user add"
// command and checks that it has the correct form, returning the
// interesting parts. The .jenv filename will be an empty string when
// it wasn't included in the output.
func parseUserAddStdout(c *gc.C, ctx *cmd.Context) (user string, password string, filename string) {
	stdout := strings.TrimSpace(ctx.Stdout.(*bytes.Buffer).String())
	lines := strings.Split(stdout, "\n")
	c.Assert(len(lines), jc.LessThan, 3)

	reLine0 := regexp.MustCompile(`^user "(.+)" added with password "(.+)"$`)
	line0Matches := reLine0.FindStringSubmatch(lines[0])
	c.Assert(len(line0Matches), gc.Equals, 3)
	user, password = line0Matches[1], line0Matches[2]

	if len(lines) == 2 {
		reLine1 := regexp.MustCompile(`^environment file written to (.+)$`)
		line1Matches := reLine1.FindStringSubmatch(lines[1])
		c.Assert(len(line1Matches), gc.Equals, 2)
		filename = line1Matches[1]
	} else {
		filename = ""
	}
	return
}
