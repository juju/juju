// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type cmdRegistrationSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdRegistrationSuite) run(c *gc.C, stdin io.Reader, args ...string) *cmd.Context {
	context := testing.Context(c)
	if stdin != nil {
		context.Stdin = stdin
	}
	command := commands.NewJujuCommand(context)
	c.Assert(testing.InitCommand(command, args), jc.ErrorIsNil)
	c.Assert(command.Run(context), jc.ErrorIsNil)
	loggo.RemoveWriter("warning") // remove logger added by main command
	return context
}

func (s *cmdRegistrationSuite) TestAddUserAndRegister(c *gc.C) {
	c.Assert(modelcmd.WriteCurrentController("dummymodel"), jc.ErrorIsNil)

	// First, add user "bob", and record the "juju register" command
	// that is printed out.
	context := s.run(c, nil, "add-user", "bob", "Bob Dobbs")
	c.Check(testing.Stderr(context), gc.Equals, "")
	stdout := testing.Stdout(context)
	c.Check(stdout, gc.Matches, `
User "Bob Dobbs \(bob\)" added
Please send this command to bob:
    juju register .*
`[1:])
	jujuRegisterCommand := strings.Fields(strings.TrimSpace(
		stdout[strings.Index(stdout, "juju register"):],
	))
	c.Logf("%q", jujuRegisterCommand)

	// Now run the "juju register" command. We need to pass the
	// controller name and password to set.
	stdin := strings.NewReader("bob-controller\nhunter2\nhunter2\n")
	args := jujuRegisterCommand[1:] // drop the "juju"
	context = s.run(c, stdin, args...)
	c.Check(testing.Stdout(context), gc.Equals, "")
	c.Check(testing.Stderr(context), gc.Equals, `
Please set a name for this controller: 
Enter password: 
Confirm password: 

Welcome, bob. You are now logged into "bob-controller".
`[1:])

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	api, err := juju.NewAPIConnection(s.ControllerStore, "bob-controller", "bob@local", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api.Close(), jc.ErrorIsNil)
}
