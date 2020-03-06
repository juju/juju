// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io"
	"regexp"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	cookiejar "github.com/juju/persistent-cookiejar"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
)

type cmdRegistrationSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdRegistrationSuite) TestAddUserAndRegister(c *gc.C) {
	// First, add user "bob", and record the "juju register" command
	// that is printed out.

	context := run(c, nil, "add-user", "bob", "Bob Dobbs")
	c.Check(cmdtesting.Stderr(context), gc.Equals, "")
	stdout := cmdtesting.Stdout(context)
	expectPat := `
User "Bob Dobbs \(bob\)" added
Please send this command to bob:
    juju register (.+)

"Bob Dobbs \(bob\)" has not been granted access to any models(.|\n)*
`[1:]
	c.Assert(stdout, gc.Matches, expectPat)

	arg := regexp.MustCompile("^" + expectPat + "$").FindStringSubmatch(stdout)[1]
	c.Logf("juju register %q", arg)

	// Now run the "juju register" command. We need to pass the
	// controller name and password to set, and we need a different
	// file store to mimic a different local OS user.
	s.CreateUserHome(c, &jujutesting.UserHomeParams{
		Username: "bob",
	})

	// The expected prompt does not include a warning about the controller
	// name, as this new local user does not have a controller named
	// "kontroll" registered.
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[kontroll\]: »bob-controller
Initial password successfully set for bob.

Welcome, bob. You are now logged into "bob-controller".

There are no models available. (.|\n)*
`[1:])

	run(c, prompter, "register", arg)
	prompter.AssertDone()

	// Make sure that the saved server details are sufficient to connect
	// to the api server.
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: jujuclient.JujuCookiePath("bob-controller"),
	})
	c.Assert(err, jc.ErrorIsNil)
	dialOpts := api.DefaultDialOpts()
	dialOpts.BakeryClient = httpbakery.NewClient()
	dialOpts.BakeryClient.Jar = jar
	accountDetails, err := s.ControllerStore.AccountDetails("bob-controller")
	c.Assert(err, jc.ErrorIsNil)
	api, err := juju.NewAPIConnection(juju.NewAPIConnectionParams{
		Store:          s.ControllerStore,
		ControllerName: "bob-controller",
		AccountDetails: accountDetails,
		DialOpts:       dialOpts,
		OpenAPI:        api.Open,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api.Close(), jc.ErrorIsNil)
}

// run runs a juju command with the given arguments.
// If stdio is given, it will be used for all input and output
// to the command; otherwise cmdtesting.Context will be used.
//
// It returns the context used to run the command.
func run(c *gc.C, stdio io.ReadWriter, args ...string) *cmd.Context {
	var context *cmd.Context
	if stdio != nil {
		context = &cmd.Context{
			Dir:    c.MkDir(),
			Stdin:  stdio,
			Stdout: stdio,
			Stderr: stdio,
		}
	} else {
		context = cmdtesting.Context(c)
	}
	command := commands.NewJujuCommand(context, "")
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	err := command.Run(context)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("stderr: %q", context.Stderr))
	loggo.RemoveWriter("warning") // remove logger added by main command
	return context
}
