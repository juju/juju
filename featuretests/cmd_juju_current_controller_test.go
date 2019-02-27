// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/commands"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
)

type cmdCurrentControllerSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdCurrentControllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.ControllerStore.AddController("ctrlOne", jujuclient.ControllerDetails{
		ControllerUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00e",
	})
	s.ControllerStore.AddController("ctrlTwo", jujuclient.ControllerDetails{
		ControllerUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00f",
	})
	s.ControllerStore.AddController("ctrlThree", jujuclient.ControllerDetails{
		ControllerUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00c",
	})

	s.ControllerStore.SetCurrentController("ctrlThree")
	s.writeStoreFiles(c)
}

func (s *cmdCurrentControllerSuite) writeStoreFiles(c *gc.C) {
	all, err := s.ControllerStore.AllControllers()
	c.Assert(err, jc.ErrorIsNil)
	current, err := s.ControllerStore.CurrentController()
	c.Assert(err, jc.ErrorIsNil)
	err = jujuclient.WriteControllersFile(&jujuclient.Controllers{
		Controllers:       all,
		CurrentController: current,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cmdCurrentControllerSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context)
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	err := command.Run(context)
	loggo.RemoveWriter("warning")
	return context, err
}

func (s *cmdCurrentControllerSuite) TestControllerListCommand(c *gc.C) {
	context, err := s.run(c, "list-controllers")
	c.Assert(err, jc.ErrorIsNil)

	expectedOutput := `
Use --refresh option with this command to see the latest information.

Controller  Model       User   Access     Cloud/Region        Models  Machines  HA  Version
ctrlOne     -           -      -                                   -         -   -  (unknown)  
ctrlThree*  -           -      -                                   -         -   -  (unknown)  
ctrlTwo     -           -      -                                   -         -   -  (unknown)  
kontroll    controller  admin  superuser  dummy/dummy-region       1         -   -  (unknown)  

`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expectedOutput)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

// TestControllerLevelCommandModelEnvVarPrecedence ensures that
// when $JUJU_CONTROLLER and $JUJU_MODEL point to different controllers,
// $JUJU_MODEL takes precedence for controller-level commands.
func (s *cmdCurrentControllerSuite) TestControllerLevelCommandModelEnvVarPrecedence(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlTwo")
	s.PatchEnvironment("JUJU_MODEL", "kontroll:")
	context, _ := s.run(c, "models")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "Controller: kontroll\n")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

// TestControllerLevelCommandOptionPrecedence ensures that
// when $JUJU_CONTROLLER and $JUJU_MODEL point to the same controller
// but a different controller is specified with -c on the command line,
// controller-level commands use command option.
func (s *cmdCurrentControllerSuite) TestControllerLevelCommandOptionPrecedence(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlOne")
	s.PatchEnvironment("JUJU_MODEL", "ctrlOne:")
	context, _ := s.run(c, "models", "-c", "kontroll")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "Controller: kontroll\n")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}
