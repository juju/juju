// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

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
	s.PatchValue(&getEnvironInfoWriter, func(c *UserChangePasswordCommand) (EnvironInfoCredsWriter, error) {
		return s.mockEnvironInfo, nil
	})
	s.PatchValue(&getConnectionCredentials, func(c *UserChangePasswordCommand) (configstore.APICredentials, error) {
		return s.mockEnvironInfo.creds, nil
	})
}

func newUserChangePassword() cmd.Command {
	return envcmd.Wrap(&UserChangePasswordCommand{})
}

func (s *UserChangePasswordCommandSuite) TestExtraArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserChangePassword(), "--foobar")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: --foobar")
}

func (s *UserChangePasswordCommandSuite) TestGenerateAndPassword(c *gc.C) {
	_, err := testing.RunCommand(c, newUserChangePassword(), "--password", "foobar", "--generate")
	c.Assert(err, gc.ErrorMatches, "You need to choose a password or generate one")
}

func (s *UserChangePasswordCommandSuite) TestFailedToReadInfo(c *gc.C) {
	s.PatchValue(&getEnvironInfoWriter, func(c *UserChangePasswordCommand) (EnvironInfoCredsWriter, error) {
		return s.mockEnvironInfo, errors.New("something failed")
	})
	_, err := testing.RunCommand(c, newUserChangePassword(), "--password", "new-password")
	c.Assert(err, gc.ErrorMatches, "something failed")
}

func (s *UserChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, err := testing.RunCommand(c, newUserChangePassword(), "--password", "new-password")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "")
	c.Assert(s.mockAPI.password, gc.Equals, "new-password")
	expected := fmt.Sprintf("your password has been updated")
	c.Assert(testing.Stdout(context), gc.Equals, expected+"\n")
}

func (s *UserChangePasswordCommandSuite) TestChangePasswordGenerate(c *gc.C) {
	context, err := testing.RunCommand(c, newUserChangePassword(), "--generate")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "")
	c.Assert(len(s.mockAPI.password) > 0, gc.Equals, true)
	expected := fmt.Sprintf("your password has been updated")
	c.Assert(testing.Stdout(context), gc.Equals, expected+"\n")
}

func (s *UserChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	s.mockAPI.failOps = []bool{true, false}
	context, err := testing.RunCommand(c, newUserChangePassword(), "--password", "new-password")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	c.Assert(s.mockAPI.username, gc.Equals, "")
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

// The first write fails, so we try to revert the password which succeeds
func (s *UserChangePasswordCommandSuite) TestRevertPasswordAfterFailedWrite(c *gc.C) {
	s.PatchValue(&getEnvironInfoWriter, func(c *UserChangePasswordCommand) (EnvironInfoCredsWriter, error) {
		return s.mockEnvironInfo, nil
	})
	// Set the password to something known
	context, err := testing.RunCommand(c, newUserChangePassword(), "--password", "password")
	c.Assert(err, gc.IsNil)

	// Fail to Write the new jenv file
	s.mockEnvironInfo.failMessage = "failed to write"
	context, err = testing.RunCommand(c, newUserChangePassword(), "--password", "new-password")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(context), gc.Equals, `Updating the jenv file failed, reverting to original password
your password has not changed
`)
	c.Assert(s.mockAPI.password, gc.Equals, "password")
}

// SetPassword api works the first time, but the write fails, our second call to set password fails
func (s *UserChangePasswordCommandSuite) TestChangePasswordRevertApiFails(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	s.mockEnvironInfo.failMessage = "failed to write"
	s.mockAPI.failOps = []bool{false, true}
	context, err := testing.RunCommand(c, newUserChangePassword(), "--password", "new-password")
	c.Assert(testing.Stderr(context), gc.Equals, `Updating the jenv file failed, reverting to original password
Updating the jenv file failed, reverting failed, you will need to edit your environments file by hand (location)
`)
	c.Assert(err, gc.ErrorMatches, "failed to do something")
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

func (m *mockEnvironInfo) Location() string {
	return "location"
}

type mockChangePasswordAPI struct {
	failMessage string
	currentOp   int
	failOps     []bool // Can be used to make the call pass/ fail in a known order
	username    string
	password    string
}

func (m *mockChangePasswordAPI) SetPassword(username, password string) error {
	if len(m.failOps) > 0 && m.failOps[m.currentOp] {
		m.currentOp++
		return errors.New(m.failMessage)
	}
	m.currentOp++
	m.username = username
	m.password = password
	return nil
}

func (*mockChangePasswordAPI) Close() error {
	return nil
}
