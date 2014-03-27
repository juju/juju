// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io/ioutil"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/environs/info"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere
// This suite provides basic tests for the AddUser command
type AddUserSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AddUserSuite{})

func (s *AddUserSuite) Testadduser(c *gc.C) {

	_, err := testing.RunCommand(c, &AddUserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &AddUserCommand{}, []string{"foobar", "newpassword"})
	c.Assert(err, gc.ErrorMatches, "Failed to create user: user already exists")
}

func (s *AddUserSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, &AddUserCommand{}, []string{"foobar", "password", "whoops"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *AddUserSuite) TestNotEnoughArgs(c *gc.C) {
	_, err := testing.RunCommand(c, &AddUserCommand{}, []string{})
	c.Assert(err, gc.ErrorMatches, `no username supplied`)
}

func (s *AddUserSuite) TestJenvOutput(c *gc.C) {
	expected := info.EnvironInfo{
		User:         "foobar",
		Password:     "password",
		StateServers: []string{},
		CACert:       "",
	}
	expectedStr, err := goyaml.Marshal(expected)
	c.Assert(err, gc.IsNil)
	tempFile, err := ioutil.TempFile("", "adduser-test")
	tempFile.Close()
	c.Assert(err, gc.IsNil)
	_, err = testing.RunCommand(c, &AddUserCommand{}, []string{"foobar", "password", "-o", tempFile.Name()})
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadFile(tempFile.Name())
	c.Assert(string(data), gc.DeepEquals, string(expectedStr))
}
