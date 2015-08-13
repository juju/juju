// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

type ChangePasswordCommandSuite struct {
	BaseSuite
	mockAPI         *mockChangePasswordAPI
	mockEnvironInfo *mockEnvironInfo
	randomPassword  string
	serverFilename  string
}

var _ = gc.Suite(&ChangePasswordCommandSuite{})

func (s *ChangePasswordCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.mockEnvironInfo = &mockEnvironInfo{
		creds: configstore.APICredentials{"user-name", "password"},
	}
	s.randomPassword = ""
	s.serverFilename = ""
	s.PatchValue(user.RandomPasswordNotify, func(pwd string) {
		s.randomPassword = pwd
	})
	s.PatchValue(user.ServerFileNotify, func(filename string) {
		s.serverFilename = filename
	})
}

func (s *ChangePasswordCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	changePasswordCommand := envcmd.WrapSystem(user.NewChangePasswordCommand(s.mockAPI, s.mockEnvironInfo))
	return testing.RunCommand(c, changePasswordCommand, args...)
}

func (s *ChangePasswordCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		outPath     string
		generate    bool
		errorString string
	}{
		{
		// no args is fine
		}, {
			args:     []string{"--generate"},
			generate: true,
		}, {
			args:     []string{"foobar"},
			user:     "foobar",
			generate: true,
			outPath:  "foobar.server",
		}, {
			args:     []string{"foobar", "--generate"},
			user:     "foobar",
			generate: true,
			outPath:  "foobar.server",
		}, {
			args:     []string{"foobar", "--output", "somefile"},
			user:     "foobar",
			generate: true,
			outPath:  "somefile",
		}, {
			args:        []string{"--foobar"},
			errorString: "flag provided but not defined: --foobar",
		}, {
			args:        []string{"foobar", "extra"},
			errorString: `unrecognized args: \["extra"\]`,
		}, {
			args:        []string{"--output", "somefile"},
			errorString: "output is only a valid option when changing another user's password",
		},
	} {
		c.Logf("test %d", i)
		command := &user.ChangePasswordCommand{}
		err := testing.InitCommand(command, test.args)
		if test.errorString == "" {
			c.Check(command.User, gc.Equals, test.user)
			c.Check(command.OutPath, gc.Equals, test.outPath)
			c.Check(command.Generate, gc.Equals, test.generate)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *ChangePasswordCommandSuite) assertRandomPassword(c *gc.C) {
	c.Assert(s.mockAPI.password, gc.Equals, s.randomPassword)
	c.Assert(s.mockAPI.password, gc.HasLen, 24)
}

func (s *ChangePasswordCommandSuite) assertPasswordFromReadPass(c *gc.C) {
	c.Assert(s.mockAPI.password, gc.Equals, "sekrit")
}

func (s *ChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "user-name")
	s.assertPasswordFromReadPass(c)
	expected := `
password: 
type password again: 
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "Your password has been updated.\n")
}

func (s *ChangePasswordCommandSuite) TestChangePasswordGenerate(c *gc.C) {
	context, err := s.run(c, "--generate")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "user-name")
	s.assertRandomPassword(c)
	c.Assert(testing.Stderr(context), gc.Equals, "Your password has been updated.\n")
}

func (s *ChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	s.mockAPI.failOps = []bool{true, false}
	_, err := s.run(c, "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	c.Assert(s.mockAPI.username, gc.Equals, "")
}

// The first write fails, so we try to revert the password which succeeds
func (s *ChangePasswordCommandSuite) TestRevertPasswordAfterFailedWrite(c *gc.C) {
	// Fail to Write the new jenv file
	s.mockEnvironInfo.failMessage = "failed to write"
	_, err := s.run(c, "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to write new password to environments file: failed to write")
	// Last api call was to set the password back to the original.
	c.Assert(s.mockAPI.password, gc.Equals, "password")
}

// SetPassword api works the first time, but the write fails, our second call to set password fails
func (s *ChangePasswordCommandSuite) TestChangePasswordRevertApiFails(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	s.mockEnvironInfo.failMessage = "failed to write"
	s.mockAPI.failOps = []bool{false, true}
	_, err := s.run(c, "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to set password back: failed to do something")
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPassword(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	context, err := s.run(c, "other")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "other")
	s.assertRandomPassword(c)
	s.assertServerFileMatches(c, s.serverFilename, "other", s.randomPassword)
	expected := `
server file written to .*other.server
`[1:]
	c.Assert(testing.Stderr(context), gc.Matches, expected)
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPasswordWithFile(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	filename := filepath.Join(c.MkDir(), "test.result")
	_, err := s.run(c, "other", "-o", filename)
	c.Assert(err, jc.ErrorIsNil)
	s.assertRandomPassword(c)
	c.Assert(filepath.Base(s.serverFilename), gc.Equals, "test.result")
	s.assertServerFileMatches(c, s.serverFilename, "other", s.randomPassword)
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
