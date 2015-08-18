package context_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
)

type launchCmdSuite struct {
	commandSuite
}

var _ = gc.Suite(&launchCmdSuite{})

func (s *launchCmdSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)
}

func (s *launchCmdSuite) TestInitReturnsNoErr(c *gc.C) {
	cmd, err := context.NewProcLaunchCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *launchCmdSuite) TestInitInvalidArgsReturnsErr(c *gc.C) {
	cmd, err := context.NewProcLaunchCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Init([]string{"mock-name", "invalid-arg"})
	c.Assert(err, gc.NotNil)
	c.Check(
		err.Error(),
		gc.Equals,
		`unrecognized args: ["invalid-arg"]`,
	)
}

func (s *launchCmdSuite) TestRun(c *gc.C) {
	// TODO(ericsnow) Setting these to empty maps should not be necessary.
	s.proc.Process.TypeOptions = map[string]string{}
	s.proc.Process.EnvVars = map[string]string{}

	plugin := &stubPlugin{stub: s.Stub}
	plugin.details = process.Details{
		ID: "id",
		Status: process.PluginStatus{
			State: "foo",
		},
	}
	s.compCtx.plugin = plugin

	cmd, err := context.NewProcLaunchCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "process-launch", cmd)
	s.setMetadata(s.proc)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)
	s.Stub.ResetCalls()

	s.checkRun(c, "", "")
	s.Stub.CheckCallNames(c, "List", "ListDefinitions", "Plugin", "Launch", "Set", "Flush")
	c.Check(s.Stub.Calls()[2].Args, jc.DeepEquals, []interface{}{&s.proc})
	c.Check(s.Stub.Calls()[3].Args, jc.DeepEquals, []interface{}{s.proc.Process})
}

func (s *launchCmdSuite) TestRunCantFindPlugin(c *gc.C) {
	plugin := &stubPlugin{stub: s.Stub}
	failure := errors.NotFoundf("mock-error")
	s.compCtx.plugin = plugin

	cmd, err := context.NewProcLaunchCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "process-launch", cmd)
	s.setMetadata(s.proc)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	s.Stub.ResetCalls()
	s.Stub.SetErrors(nil, nil, failure)

	err = s.cmd.Run(s.cmdCtx)
	c.Assert(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "List", "ListDefinitions", "Plugin")
}

func (s *launchCmdSuite) TestLaunchCommandErrorRunning(c *gc.C) {
	plugin := &stubPlugin{stub: s.Stub}
	failure := errors.Errorf("mock-error")
	s.compCtx.plugin = plugin

	cmd, err := context.NewProcLaunchCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "process-launch", cmd)
	s.setMetadata(s.proc)

	err = cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)
	s.Stub.ResetCalls()
	s.Stub.SetErrors(nil, nil, nil, failure)

	err = cmd.Run(s.cmdCtx)
	c.Assert(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "List", "ListDefinitions", "Plugin", "Launch")
}
