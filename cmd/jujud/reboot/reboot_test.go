package reboot

import (
	"errors"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/apiserver/params"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { gc.TestingT(t) }

type RebootSuite struct{}

var _ = gc.Suite(&RebootSuite{})

func (s *RebootSuite) TestExecuteReboot_ShouldDoNothingReturns(c *gc.C) {
	// If this doesn't return immediately, it would panic
	ExecuteReboot(nil, nil, 0, params.ShouldDoNothing, nil)
}

func (s *RebootSuite) TestExecuteReboot_RebootStateFail(c *gc.C) {
	openRebootState := func() (reboot.State, error) {
		return nil, errors.New("foo")
	}
	err := ExecuteReboot(nil, openRebootState, 0, params.ShouldReboot, nil)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "foo")
}

func (s *RebootSuite) TestExecuteReboot_RebootOKReturnsErr(c *gc.C) {
	openRebootState := func() (reboot.State, error) {
		return nil, nil
	}
	rebootOK := make(chan error)
	go func() { rebootOK <- errors.New("foo") }()
	err := ExecuteReboot(nil, openRebootState, 0, params.ShouldReboot, rebootOK)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "foo")
}

func (s *RebootSuite) TestExecuteReboot_ExecuteBuiltShutdownCmd(c *gc.C) {
	openRebootState := func() (reboot.State, error) {
		return nil, nil
	}
	rebootOK := make(chan error)
	go func() { rebootOK <- errors.New("foo") }()
	err := ExecuteReboot(nil, openRebootState, 0, params.ShouldReboot, rebootOK)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "foo")
}
