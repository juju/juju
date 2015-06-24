package context_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process/context"
	"github.com/juju/juju/process/plugin"
)

type launchCmdSuite struct {
	commandSuite
}

var _ = gc.Suite(&launchCmdSuite{})

func (s *launchCmdSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)
}

func (s *launchCmdSuite) TestInitReturnsNoErr(c *gc.C) {
	cmd, err := context.NewProcLaunchCommand(nil, nil, s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *launchCmdSuite) TestInitInvalidArgsReturnsErr(c *gc.C) {
	cmd, err := context.NewProcLaunchCommand(nil, nil, s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Init([]string{"mock-name", "invalid-arg"})
	c.Assert(err, gc.NotNil)
	c.Check(
		err.Error(),
		gc.Equals,
		fmt.Sprintf("expected %s, got [mock-name invalid-arg]", cmd.Info().Args),
	)
}

func (s *launchCmdSuite) TestRun(c *gc.C) {

	var mockPlugin *plugin.Plugin

	numLaunchPluginCalls := 0
	launchPlugin := func(p plugin.Plugin, process charm.Process) (plugin.ProcDetails, error) {
		numLaunchPluginCalls++
		c.Check(p, gc.DeepEquals, *mockPlugin)
		return plugin.ProcDetails{"id", plugin.ProcStatus{"foo"}}, nil
	}

	numFindPluginCalls := 0
	findPlugin := func(pluginName string) (*plugin.Plugin, error) {
		numFindPluginCalls++
		mockPlugin = &plugin.Plugin{
			Name: pluginName,
			Path: "mock-path",
		}
		return mockPlugin, nil
	}

	cmd, err := context.NewProcLaunchCommand(findPlugin, launchPlugin, s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "launch", cmd)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	s.checkRun(c, "", "")
	c.Check(numLaunchPluginCalls, gc.Equals, 1)
	c.Check(numFindPluginCalls, gc.Equals, 1)
}

func (s *launchCmdSuite) TestRunCantFindPlugin(c *gc.C) {

	findPlugin := func(pluginName string) (*plugin.Plugin, error) {
		return nil, fmt.Errorf("mock-error")
	}

	cmd, err := context.NewProcLaunchCommand(findPlugin, nil, s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "launch", cmd)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	err = s.cmd.Run(s.cmdCtx)
	c.Assert(err, gc.ErrorMatches, "mock-error")
}

func (s *launchCmdSuite) TestLaunchCommandErrorRunning(c *gc.C) {

	findPlugin := func(pluginName string) (*plugin.Plugin, error) {
		return &plugin.Plugin{}, nil
	}

	launchPlugin := func(p plugin.Plugin, process charm.Process) (plugin.ProcDetails, error) {
		return plugin.ProcDetails{}, fmt.Errorf("mock-error")
	}

	cmd, err := context.NewProcLaunchCommand(findPlugin, launchPlugin, s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "launch", cmd)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Run(s.cmdCtx)
	c.Check(err, gc.ErrorMatches, "mock-error")
}
