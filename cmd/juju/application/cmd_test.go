// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"os"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type CmdSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&CmdSuite{})

var deployTests = []struct {
	args []string
	com  *DeployCommand
}{
	{
		[]string{"charm-name"},
		&DeployCommand{},
	}, {
		[]string{"charm-name", "application-name"},
		&DeployCommand{ApplicationName: "application-name"},
	}, {
		[]string{"--num-units", "33", "charm-name"},
		&DeployCommand{UnitCommandBase: UnitCommandBase{NumUnits: 33}},
	}, {
		[]string{"-n", "104", "charm-name"},
		&DeployCommand{UnitCommandBase: UnitCommandBase{NumUnits: 104}},
	},
}

func initExpectations(com *DeployCommand, store jujuclient.ClientStore) {
	if com.CharmOrBundle == "" {
		com.CharmOrBundle = "charm-name"
	}
	if com.NumUnits == 0 {
		com.NumUnits = 1
	}
	com.SetClientStore(store)
	com.SetModelName("admin")
}

func initDeployCommand(store jujuclient.ClientStore, args ...string) (*DeployCommand, error) {
	com := &DeployCommand{}
	com.SetClientStore(store)
	return com, coretesting.InitCommand(modelcmd.Wrap(com), args)
}

func (s *CmdSuite) TestDeployCommandInit(c *gc.C) {
	for i, t := range deployTests {
		c.Logf("\ntest %d: args %q", i, t.args)
		initExpectations(t.com, s.ControllerStore)
		com, err := initDeployCommand(s.ControllerStore, t.args...)
		// Testing that the flag set is populated is good enough for the scope
		// of this test.
		c.Assert(com.flagSet, gc.NotNil)
		com.flagSet = nil
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(com, jc.DeepEquals, t.com)
	}

	// test relative --config path
	ctx := coretesting.Context(c)
	expected := []byte("test: data")
	path := ctx.AbsPath("testconfig.yaml")
	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	_, err = file.Write(expected)
	c.Assert(err, jc.ErrorIsNil)
	file.Close()

	com, err := initDeployCommand(s.ControllerStore, "--config", "testconfig.yaml", "charm-name")
	c.Assert(err, jc.ErrorIsNil)
	actual, err := com.Config.Read(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expected, gc.DeepEquals, actual)

	// missing args
	_, err = initDeployCommand(s.ControllerStore)
	c.Assert(err, gc.ErrorMatches, "no charm or bundle specified")

	// bad unit count
	_, err = initDeployCommand(s.ControllerStore, "charm-name", "--num-units", "0")
	c.Assert(err, gc.ErrorMatches, "--num-units must be a positive integer")
	_, err = initDeployCommand(s.ControllerStore, "charm-name", "-n", "0")
	c.Assert(err, gc.ErrorMatches, "--num-units must be a positive integer")

	// environment tested elsewhere
}

func initExposeCommand(args ...string) (*exposeCommand, error) {
	com := &exposeCommand{}
	return com, coretesting.InitCommand(modelcmd.Wrap(com), args)
}

func (*CmdSuite) TestExposeCommandInit(c *gc.C) {
	// missing args
	_, err := initExposeCommand()
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// environment tested elsewhere
}

func initUnexposeCommand(args ...string) (*unexposeCommand, error) {
	com := &unexposeCommand{}
	return com, coretesting.InitCommand(modelcmd.Wrap(com), args)
}

func (*CmdSuite) TestUnexposeCommandInit(c *gc.C) {
	// missing args
	_, err := initUnexposeCommand()
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// environment tested elsewhere
}

func initRemoveUnitCommand(args ...string) (cmd.Command, error) {
	com := NewRemoveUnitCommand()
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestRemoveUnitCommandInit(c *gc.C) {
	// missing args
	_, err := initRemoveUnitCommand()
	c.Assert(err, gc.ErrorMatches, "no units specified")
	// not a unit
	_, err = initRemoveUnitCommand("seven/nine")
	c.Assert(err, gc.ErrorMatches, `invalid unit name "seven/nine"`)
}
