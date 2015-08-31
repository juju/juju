package context_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

type launchCmdSuite struct {
	commandSuite
}

var _ = gc.Suite(&launchCmdSuite{})

func (s *launchCmdSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewWorkloadLaunchCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	cmd.ReadDefinitions = s.readDefinitions
	s.setCommand(c, "workload-launch", cmd)
}

func (s *launchCmdSuite) TestInitReturnsNoErr(c *gc.C) {
	err := s.cmd.Init([]string{s.workload.Name})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *launchCmdSuite) TestInitInvalidArgsReturnsErr(c *gc.C) {
	err := s.cmd.Init([]string{"mock-name", "invalid-arg"})
	c.Assert(err, gc.NotNil)
	c.Check(
		err.Error(),
		gc.Equals,
		`unrecognized args: ["invalid-arg"]`,
	)
}

func (s *launchCmdSuite) TestRun(c *gc.C) {
	// TODO(ericsnow) Setting these to empty maps should not be necessary.
	s.workload.Workload.TypeOptions = map[string]string{}
	s.workload.Workload.EnvVars = map[string]string{}

	plugin := &stubPlugin{stub: s.Stub}
	plugin.details = workload.Details{
		ID: "id",
		Status: workload.PluginStatus{
			State: "foo",
		},
	}
	s.compCtx.plugin = plugin
	s.setMetadata(s.workload)

	err := s.cmd.Init([]string{s.workload.Name})
	c.Assert(err, jc.ErrorIsNil)
	s.Stub.ResetCalls()

	s.checkRun(c, "", "")
	s.Stub.CheckCallNames(c, "List", "Plugin", "Launch", "Track", "Flush")
	c.Check(s.Stub.Calls()[1].Args, jc.DeepEquals, []interface{}{&s.workload, ""})
	c.Check(s.Stub.Calls()[2].Args, jc.DeepEquals, []interface{}{s.workload.Workload})
}

func (s *launchCmdSuite) TestRunCantFindPlugin(c *gc.C) {
	plugin := &stubPlugin{stub: s.Stub}
	failure := errors.NotFoundf("mock-error")
	s.compCtx.plugin = plugin

	s.setMetadata(s.workload)

	err := s.cmd.Init([]string{s.workload.Name})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	s.Stub.ResetCalls()
	s.Stub.SetErrors(nil, failure)

	err = s.cmd.Run(s.cmdCtx)
	c.Assert(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "List", "Plugin")
}

func (s *launchCmdSuite) TestLaunchCommandErrorRunning(c *gc.C) {
	plugin := &stubPlugin{stub: s.Stub}
	failure := errors.Errorf("mock-error")
	s.compCtx.plugin = plugin

	s.setMetadata(s.workload)

	err := s.cmd.Init([]string{s.workload.Name})
	c.Assert(err, jc.ErrorIsNil)
	s.Stub.ResetCalls()
	s.Stub.SetErrors(nil, nil, failure)

	err = s.cmd.Run(s.cmdCtx)
	c.Assert(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "List", "Plugin", "Launch")
}
