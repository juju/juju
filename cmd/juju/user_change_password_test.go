// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "user add" command
type UserChangePasswordCommandSuite struct {
	//testing.FakeJujuHomeSuite
	jujutesting.JujuConnSuite
	mockAPI *mockChangePasswordAPI
}

var _ = gc.Suite(&UserChangePasswordCommandSuite{})

func (s *UserChangePasswordCommandSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.PatchValue(&getChangePasswordAPI, func(c *UserChangePasswordCommand) (ChangePasswordAPI, error) {
		return s.mockAPI, nil
	})
}

func newUserChangePassword() cmd.Command {
	return envcmd.Wrap(&UserChangePasswordCommand{})
}

func (s *UserChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	_, err := environs.PrepareFromName("dummyenv", nullContext(c), s.ConfigStore)
	c.Assert(err, gc.IsNil)
	context, err := testing.RunCommand(c, newUserChangePassword(), "new-password")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "")
	c.Assert(s.mockAPI.password, gc.Equals, "new-password")
	expected := fmt.Sprintf("your password has been updated")
	c.Assert(testing.Stdout(context), gc.Equals, expected+"\n")
}

func (s *UserChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	_, err := environs.PrepareFromName("dummyenv", nullContext(c), s.ConfigStore)
	c.Assert(err, gc.IsNil)
	context, err := testing.RunCommand(c, newUserChangePassword(), "new-password")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	c.Assert(s.mockAPI.username, gc.Equals, "")
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

type mockChangePasswordAPI struct {
	failMessage string
	username    string
	password    string
}

func (m *mockChangePasswordAPI) SetPassword(username, password string) error {
	m.username = username
	m.password = password
	if m.failMessage == "" {
		return nil
	}
	return errors.New(m.failMessage)
}

func (*mockChangePasswordAPI) Close() error {
	return nil
}
