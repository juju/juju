// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type cmdLoginSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdLoginSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	os.Setenv(osenv.JujuModelEnvKey, "")
}

func (s *cmdLoginSuite) run(c *gc.C, stdin io.Reader, args ...string) *cmd.Context {
	context := testing.Context(c)
	if stdin != nil {
		context.Stdin = stdin
	}
	command := commands.NewJujuCommand(context)
	c.Assert(testing.InitCommand(command, args), jc.ErrorIsNil)
	err := command.Run(context)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("stdout: %q; stderr: %q", context.Stdout, context.Stderr))
	loggo.RemoveWriter("warning") // remove logger added by main command
	return context
}

func (s *cmdLoginSuite) createTestUser(c *gc.C) {
	s.run(c, nil, "add-user", "test")
	s.run(c, nil, "grant", "test", "read", "controller")
	s.changeUserPassword(c, "test", "hunter2")
}

func (s *cmdLoginSuite) changeUserPassword(c *gc.C, user, password string) {
	s.run(c, strings.NewReader(password+"\n"+password+"\n"), "change-user-password", user)
}

func (s *cmdLoginSuite) TestLoginCommand(c *gc.C) {
	s.createTestUser(c)

	// logout "admin" first; we'll need to give it
	// a non-random password before we can do so.
	s.changeUserPassword(c, "admin", "hunter2")
	s.run(c, nil, "logout")

	context := s.run(c, strings.NewReader("hunter2\nhunter2\n"), "login", "test")
	c.Assert(testing.Stdout(context), gc.Equals, "")
	c.Assert(testing.Stderr(context), gc.Equals, `
password: 
You are now logged in to "kontroll" as "test@local".
`[1:])

	// We should have a macaroon, but no password, in the client store.
	store := jujuclient.NewFileClientStore()
	accountDetails, err := store.AccountDetails("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accountDetails.Password, gc.Equals, "")
	c.Assert(accountDetails.Macaroon, gc.Not(gc.Equals), "")

	// We should be able to login with the macaroon.
	s.run(c, nil, "status")
}
