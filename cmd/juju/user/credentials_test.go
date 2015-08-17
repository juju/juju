// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

type CredentialsCommandSuite struct {
	BaseSuite
	serverFilename string
}

var _ = gc.Suite(&CredentialsCommandSuite{})

func (s *CredentialsCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.serverFilename = ""
	s.PatchValue(user.ServerFileNotify, func(filename string) {
		s.serverFilename = filename
	})
}

func (s *CredentialsCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := envcmd.WrapSystem(&user.CredentialsCommand{})
	return testing.RunCommand(c, command, args...)
}

func (s *CredentialsCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		outPath     string
		errorString string
	}{
		{
		// no args is fine
		}, {
			args:    []string{"--output=foo.bar"},
			outPath: "foo.bar",
		}, {
			args:    []string{"-o", "foo.bar"},
			outPath: "foo.bar",
		}, {
			args:        []string{"foobar"},
			errorString: `unrecognized args: \["foobar"\]`,
		},
	} {
		c.Logf("test %d", i)
		command := &user.CredentialsCommand{}
		err := testing.InitCommand(command, test.args)
		if test.errorString == "" {
			c.Check(command.OutPath, gc.Equals, test.outPath)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *CredentialsCommandSuite) TestNoArgs(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	// User and password are set in BaseSuite.SetUpTest.
	s.assertServerFileMatches(c, s.serverFilename, "user-test", "password")
	expected := `
server file written to .*user-test.server
`[1:]
	c.Assert(testing.Stderr(context), gc.Matches, expected)
}

func (s *CredentialsCommandSuite) TestFilename(c *gc.C) {
	context, err := s.run(c, "--output=testing.creds")
	c.Assert(err, jc.ErrorIsNil)
	// User and password are set in BaseSuite.SetUpTest.
	s.assertServerFileMatches(c, s.serverFilename, "user-test", "password")
	expected := `
server file written to .*testing.creds
`[1:]
	c.Assert(testing.Stderr(context), gc.Matches, expected)
}
