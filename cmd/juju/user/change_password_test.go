// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
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
}

var _ = gc.Suite(&ChangePasswordCommandSuite{})

func (s *ChangePasswordCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockChangePasswordAPI{}
	s.mockEnvironInfo = &mockEnvironInfo{
		creds: configstore.APICredentials{"user-name", "password"},
	}
	s.PatchValue(user.GetChangePasswordAPI, func(c *user.ChangePasswordCommand) (user.ChangePasswordAPI, error) {
		return s.mockAPI, nil
	})
	s.PatchValue(user.GetEnvironInfoWriter, func(c *user.ChangePasswordCommand) (user.EnvironInfoCredsWriter, error) {
		return s.mockEnvironInfo, nil
	})
	s.PatchValue(user.GetConnectionCredentials, func(c *user.ChangePasswordCommand) (configstore.APICredentials, error) {
		return s.mockEnvironInfo.creds, nil
	})
}

func newUserChangePassword() cmd.Command {
	return envcmd.Wrap(&user.ChangePasswordCommand{})
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
			args:        []string{"--foobar"},
			errorString: "flag provided but not defined: --foobar",
		}, {
			args: []string{"foobar"},
			user: "foobar",
		}, {
			args:        []string{"foobar", "extra"},
			errorString: `unrecognized args: \["extra"\]`,
		}, {
			args:        []string{"--output", "somefile"},
			errorString: "output is only a valid option when changing another's password",
		}, {
			args:        []string{"-o", "somefile"},
			errorString: "output is only a valid option when changing another's password",
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

func (s *ChangePasswordCommandSuite) TestFailedToReadInfo(c *gc.C) {
	s.PatchValue(user.GetEnvironInfoWriter, func(c *user.ChangePasswordCommand) (user.EnvironInfoCredsWriter, error) {
		return s.mockEnvironInfo, errors.New("something failed")
	})
	_, err := testing.RunCommand(c, newUserChangePassword(), "--generate")
	c.Assert(err, gc.ErrorMatches, "something failed")
}

func (s *ChangePasswordCommandSuite) TestChangePassword(c *gc.C) {
	context, err := testing.RunCommand(c, newUserChangePassword())
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "user-name")
	c.Assert(s.mockAPI.password, gc.Equals, "sekrit")
	expected := `
password:
type password again:
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "Your password has been updated.\n")
}

func (s *ChangePasswordCommandSuite) TestChangePasswordGenerate(c *gc.C) {
	context, err := testing.RunCommand(c, newUserChangePassword(), "--generate")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "user-name")
	c.Assert(s.mockAPI.password, gc.Not(gc.Equals), "sekrit")
	c.Assert(s.mockAPI.password, gc.HasLen, 24)
	c.Assert(testing.Stderr(context), gc.Equals, "Your password has been updated.\n")
}

func (s *ChangePasswordCommandSuite) TestChangePasswordFail(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	s.mockAPI.failOps = []bool{true, false}
	_, err := testing.RunCommand(c, newUserChangePassword(), "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to do something")
	c.Assert(s.mockAPI.username, gc.Equals, "")
}

// The first write fails, so we try to revert the password which succeeds
func (s *ChangePasswordCommandSuite) TestRevertPasswordAfterFailedWrite(c *gc.C) {
	// Fail to Write the new jenv file
	s.mockEnvironInfo.failMessage = "failed to write"
	_, err := testing.RunCommand(c, newUserChangePassword(), "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to write new password to environments file: failed to write")
	// Last api call was to set the password back to the original.
	c.Assert(s.mockAPI.password, gc.Equals, "password")
}

// SetPassword api works the first time, but the write fails, our second call to set password fails
func (s *ChangePasswordCommandSuite) TestChangePasswordRevertApiFails(c *gc.C) {
	s.mockAPI.failMessage = "failed to do something"
	s.mockEnvironInfo.failMessage = "failed to write"
	s.mockAPI.failOps = []bool{false, true}
	_, err := testing.RunCommand(c, newUserChangePassword(), "--generate")
	c.Assert(err, gc.ErrorMatches, "failed to set password back: failed to do something")
}

func (s *ChangePasswordCommandSuite) TestChangeOthersPassword(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	context, err := testing.RunCommand(c, newUserChangePassword(), "other")
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "other")
	c.Assert(s.mockAPI.password, gc.Equals, "sekrit")
	filename := context.AbsPath("other.jenv")
	expected := `
password:
type password again:
environment file written to `[1:] + filename + "\n"
	c.Assert(testing.Stdout(context), gc.Equals, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "")
	assertJENVContents(c, context.AbsPath("other.jenv"), "other", "sekrit")

}

func (s *ChangePasswordCommandSuite) TestChangeOthersPasswordWithFile(c *gc.C) {
	// The checks for user existence and admin rights are tested
	// at the apiserver level.
	filename := filepath.Join(c.MkDir(), "test.jenv")
	context, err := testing.RunCommand(c, newUserChangePassword(), "other", "-o", filename)
	c.Assert(err, gc.IsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "other")
	c.Assert(s.mockAPI.password, gc.Equals, "sekrit")
	expected := `
password:
type password again:
environment file written to `[1:] + filename + "\n"
	c.Assert(testing.Stdout(context), gc.Equals, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "")
	assertJENVContents(c, filename, "other", "sekrit")
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
