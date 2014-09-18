package reboot_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/reboot"
)

var rebootBin = "shutdown.exe"

func (s *RebootSuite) rebootCommandParams(c *gc.C) []string {
	return []string{
		"-r",
		"-t",
		"0",
	}
}

func (s *RebootSuite) shutdownCommandParams(c *gc.C) []string {
	return []string{
		"-s",
		"-t",
		"0",
	}
}

func (s *RebootSuite) TestRebootNoContainers(c *gc.C) {
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, gc.IsNil)
	expectedRebootParams := s.rebootCommandParams(c)

	err = w.ExecuteReboot(params.ShouldReboot)
	c.Assert(err, gc.IsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
}

func (s *RebootSuite) TestShutdownNoContainers(c *gc.C) {
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, gc.IsNil)
	expectedShutdownParams := s.shutdownCommandParams(c)

	err = w.ExecuteReboot(params.ShouldShutdown)
	c.Assert(err, gc.IsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedShutdownParams...)
}
