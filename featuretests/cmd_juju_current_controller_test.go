// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"regexp"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	command := commands.NewJujuCommand(context, "")
	err := cmdtesting.InitCommand(command, args)
	if err != nil {
		return context, err
	}
	err = command.Run(context)
	loggo.RemoveWriter("warning")
	return context, err
}

func (s *cmdCurrentControllerSuite) TestControllerListCommand(c *gc.C) {
	context, err := s.run(c, "list-controllers")
	c.Assert(err, jc.ErrorIsNil)

	expectedOutput := `
Use --refresh option with this command to see the latest information.

Controller  Model       User   Access     Cloud/Region        Models  Nodes  HA  Version
ctrlOne     -           -      -                                   -      -   -  (unknown)  
ctrlThree*  -           -      -                                   -      -   -  (unknown)  
ctrlTwo     -           -      -                                   -      -   -  (unknown)  
kontroll    controller  admin  superuser  dummy/dummy-region       1      -   -  (unknown)  

`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expectedOutput)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

// TestControllerLevelCommandModelEnvVarConflict ensures that
// when $JUJU_CONTROLLER and $JUJU_MODEL point to different controllers,
// $JUJU_MODEL takes precedence for controller-level commands.
func (s *cmdCurrentControllerSuite) TestControllerLevelCommandModelEnvVarConflict(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlTwo")
	s.PatchEnvironment("JUJU_MODEL", "kontroll:")
	context, err := s.run(c, "models")
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "")
	c.Assert(cmdtesting.Stderr(context), jc.Contains, "ERROR controller name from JUJU_MODEL (kontroll) conflicts with value in JUJU_CONTROLLER (ctrlTwo)\n")
}

// TestControllerLevelCommandOptionPrecedence ensures that
// when $JUJU_CONTROLLER and $JUJU_MODEL point to the same controller
// but a different controller is specified with -c on the command line,
// controller-level commands use command option.
func (s *cmdCurrentControllerSuite) TestControllerLevelCommandOptionPrecedence(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlTwo")
	s.PatchEnvironment("JUJU_MODEL", "ctrlOne:")
	context, err := s.run(c, "models", "-c", "kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "Controller: kontroll\n")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

// TestControllerLevelCommandOptionEnvConflict ensures that
// when $JUJU_CONTROLLER and $JUJU_MODEL point to the same controller
// but a different controller is specified with -c on the command line,
// controller-level commands use command option.
func (s *cmdCurrentControllerSuite) TestControllerLevelCommandOptionEnvConflict(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlOne")
	s.PatchEnvironment("JUJU_MODEL", "ctrlTwo:")
	context, err := s.run(c, "models", "-c", "kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "Controller: kontroll\n")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}

// TestModelLevelCommandOptionPrecedenceControllerVar ensures that
// when $JUJU_CONTROLLER is set but $JUJU_MODEL is not and
// a different controller is specified with -m on the command line,
// model level commands use command option.
func (s *cmdCurrentControllerSuite) TestModelLevelCommandOptionPrecedenceControllerVar(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlTwo")
	context, err := s.run(c, "status", "-m", "kontroll:controller")
	c.Assert(err, jc.ErrorIsNil)
	// No need to check this whole output which will be of the form...
	//    Model       Controller  Cloud/Region        Version  SLA          Timestamp
	//    controller  kontroll    dummy/dummy-region  2.5.2    unsupported  15:22:42+01:00
	// It's enough to check that the model and expected controller appear.
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "controller  kontroll")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "Model \"controller\" is empty.\n")
}

// TestModelLevelCommandOptionPrecedenceModelVar ensures that
// when $JUJU_MODEL is set but $JUJU_CONTROLLER is not and
// a different controller is specified with -m on the command line,
// model level commands use command option.
func (s *cmdCurrentControllerSuite) TestModelLevelCommandOptionPrecedenceModelVar(c *gc.C) {
	s.PatchEnvironment("JUJU_MODEL", "ctrlTwo:controller")
	context, err := s.run(c, "status", "-m", "kontroll:controller")
	c.Assert(err, jc.ErrorIsNil)
	// No need to check this whole output which will be of the form...
	//    Model       Controller  Cloud/Region        Version  SLA          Timestamp
	//    controller  kontroll    dummy/dummy-region  2.5.2    unsupported  15:22:42+01:00
	// It's enough to check that the model and expected controller appear.
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "controller  kontroll")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "Model \"controller\" is empty.\n")
}

// TestModelLevelCommandOptionPrecedence ensures that
// when $JUJU_CONTROLLER and $JUJU_MODEL point to different controllers and
// -m is specified on the command line with the controller being included,
// model level command uses command option.
func (s *cmdCurrentControllerSuite) TestModelLevelCommandOptionPrecedence(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlOne")
	s.PatchEnvironment("JUJU_MODEL", "ctrlTwo:controller")
	context, err := s.run(c, "status", "-m", "kontroll:controller")
	c.Assert(err, jc.ErrorIsNil)
	// No need to check this whole output which will be of the form...
	//    Model       Controller  Cloud/Region        Version  SLA          Timestamp
	//    controller  kontroll    dummy/dummy-region  2.5.2    unsupported  15:22:42+01:00
	// It's enough to check that the model and expected controller appear.
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "controller  kontroll")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "Model \"controller\" is empty.\n")
}

// TestModelLevelCommandOptionOnly ensures that
// when -m is specified on the command line with the controller being included,
// model level command uses command option.
func (s *cmdCurrentControllerSuite) TestModelLevelCommandOptionOnly(c *gc.C) {
	context, err := s.run(c, "status", "-m", "kontroll:controller")
	c.Assert(err, jc.ErrorIsNil)
	// No need to check this whole output which will be of the form...
	//    Model       Controller  Cloud/Region        Version  SLA          Timestamp
	//    controller  kontroll    dummy/dummy-region  2.5.2    unsupported  15:22:42+01:00
	// It's enough to check that the model and expected controller appear.
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "controller  kontroll")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "Model \"controller\" is empty.\n")
}

// TestModelLevelCommandWithNoControllerInModelOptionAndNoEnvVar ensures that
// when -m is specified on the command line without the controller being included,
// model level command uses controller from latest 'switch' call.
func (s *cmdCurrentControllerSuite) TestModelLevelCommandWithNoControllerInModelOptionAndNoEnvVar(c *gc.C) {
	s.ControllerStore.SetCurrentController("kontroll")
	context, err := s.run(c, "status", "-m", "controller")
	c.Assert(err, jc.ErrorIsNil)
	// No need to check this whole output which will be of the form...
	//    Model       Controller  Cloud/Region        Version  SLA          Timestamp
	//    controller  kontroll    dummy/dummy-region  2.5.2    unsupported  15:22:42+01:00
	// It's enough to check that the model and expected controller appear.
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "controller  kontroll")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "Model \"controller\" is empty.\n")
}

// TestModelLevelCommandWithEnvVarsConflict ensures that
// when $JUJU_CONTROLLER and $JUJU_MODEL point to different controllers and
// -m is specified on the command line without the controller being included,
// model level command uses command option.
func (s *cmdCurrentControllerSuite) TestModelLevelCommandWithEnvVarsConflict(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "ctrlOne")
	s.PatchEnvironment("JUJU_MODEL", "ctrlTwo:controller")
	context, err := s.run(c, "status", "-m", "controller")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("controller name from JUJU_MODEL (ctrlTwo) conflicts with value in JUJU_CONTROLLER (ctrlOne)"))
	c.Assert(cmdtesting.Stdout(context), jc.Contains, "")
	c.Assert(cmdtesting.Stderr(context), gc.Equals, "")
}
