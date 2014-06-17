// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

type UserChangePasswordCommandSuite struct {
	testing.FakeJujuHomeSuite
	mockAPI         *mockChangePasswordAPI
	mockEnvironInfo *mockEnvironInfo
}

var _ = gc.Suite(&UserChangePasswordCommandSuite{})

func (s *UserChangePasswordCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.mockEnvironInfo = &mockEnvironInfo{}
	s.PatchValue(&getChangePasswordAPI, func(c *UserChangePasswordCommand) (ChangePasswordAPI, error) {
		return s.mockAPI, nil
	})
	s.PatchValue(&getEnvironInfo, func(c *UserChangePasswordCommand) (configstore.EnvironInfo, error) {
		return s.mockEnvironInfo, nil
	})
}

func newUserChangePassword() cmd.Command {
	return envcmd.Wrap(&UserChangePasswordCommand{})
}

func (s *UserChangePasswordCommandSuite) TestExtraArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserChangePassword(), "new-password", "--foobar")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: --foobar")
}

func (s *UserChangePasswordCommandSuite) TestFailedToReadInfo(c *gc.C) {
	s.PatchValue(&getEnvironInfo, func(c *UserChangePasswordCommand) (configstore.EnvironInfo, error) {
		return s.mockEnvironInfo, errors.New("something failed")
	})
	_, err := testing.RunCommand(c, newUserChangePassword(), "new-password")
	c.Assert(err, gc.ErrorMatches, "something failed")
}

func (s *UserChangePasswordCommandSuite) TestFailedToWriteInfo(c *gc.C) {
	s.PatchValue(&getEnvironInfo, func(c *UserChangePasswordCommand) (configstore.EnvironInfo, error) {
		return s.mockEnvironInfo, nil
	})
	s.mockEnvironInfo.failMessage = "failed to write"
	_, err := testing.RunCommand(c, newUserChangePassword(), "new-password")
	c.Assert(err, gc.ErrorMatches, "Failed to write the password back to the .jenv file: failed to write")
}

func (s *UserChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, err := testing.RunCommand(c, newUserChangePassword(), "new-password")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "")
	c.Assert(s.mockAPI.password, gc.Equals, "new-password")
	expected := fmt.Sprintf("your password has been updated")
	c.Assert(testing.Stdout(context), gc.Equals, expected+"\n")
}

func (s *UserChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	context, err := testing.RunCommand(c, newUserChangePassword(), "new-password")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	c.Assert(s.mockAPI.username, gc.Equals, "")
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

type mockEnvironInfo struct {
	failMessage string
	creds       configstore.APICredentials
}

func (m *mockEnvironInfo) Write() error {
	if m.failMessage != "" {
		return errors.New(m.failMessage)
	}
	return nil
}

func (m *mockEnvironInfo) SetAPICredentials(creds configstore.APICredentials) {
	m.creds = creds
}

func (m *mockEnvironInfo) APICredentials() configstore.APICredentials {
	return m.creds
}

func (m *mockEnvironInfo) Initialized() bool {
	return true
}

func (m *mockEnvironInfo) BootstrapConfig() map[string]interface{} {
	return nil
}

func (m *mockEnvironInfo) APIEndpoint() configstore.APIEndpoint {
	return configstore.APIEndpoint{}
}

func (m *mockEnvironInfo) SetBootstrapConfig(map[string]interface{}) {}

func (m *mockEnvironInfo) SetAPIEndpoint(configstore.APIEndpoint) {}

func (m *mockEnvironInfo) Location() string {
	return ""
}

func (m *mockEnvironInfo) Destroy() error {
	return nil
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
