package machine_test

import (
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/machine"
)

type UpgradeSeriesSuite struct{}

var _ = gc.Suite(&UpgradeSeriesSuite{})

const machineArg = "1"
const seriesArg = "xenial"

func (s *UpgradeSeriesSuite) SetUpTest(c *gc.C) {
}

func (s *UpgradeSeriesSuite) runUpgradeSeriesCommand(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, machine.NewUpgradeSeriesCommand(), args...)
	return err
}

func (s *UpgradeSeriesSuite) TestPrepareCommand(c *gc.C) {
	err := s.runUpgradeSeriesCommand(c, machine.PrepareCommand, machineArg, seriesArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestPrepareCommandShouldAcceptAgreeFlag(c *gc.C) {
	err := s.runUpgradeSeriesCommand(c, machine.PrepareCommand, machineArg, seriesArg, "--agree")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestCompleteCommand(c *gc.C) {
	err := s.runUpgradeSeriesCommand(c, machine.CompleteCommand, machineArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestCompleteCommandDoesNotAcceptSeries(c *gc.C) {
	err := s.runUpgradeSeriesCommand(c, machine.CompleteCommand, machineArg, seriesArg)
	c.Assert(err, gc.ErrorMatches, "wrong number of arguments")
}
