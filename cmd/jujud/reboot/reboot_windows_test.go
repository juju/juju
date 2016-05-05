// +build windows

package reboot

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

func (s *RebootSuite) TestBuildRebootCommand_ShouldDoNothing(c *gc.C) {
	cmd, args := buildRebootCommand(params.ShouldDoNothing, 0)
	c.Check(cmd, gc.Equals, "")
	c.Check(args, gc.HasLen, 0)
}

func (s *RebootSuite) TestBuildRebootCommand_IsShutdownCmd(c *gc.C) {
	cmd, _ := buildRebootCommand(params.ShouldReboot, 0)
	c.Check(cmd, gc.Equals, "shutdown.exe")
}

func (s *RebootSuite) TestBuildRebootCommand_SetsDelay(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 1*time.Minute)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[0], gc.Equals, "-t")
	c.Check(args[1], gc.Equals, "60")
}

func (s *RebootSuite) TestBuildRebootCommand_DelayToCeilingSecond(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 2*time.Microsecond)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[0], gc.Equals, "-t")
	c.Check(args[1], gc.Equals, "1")
}

func (s *RebootSuite) TestBuildRebootCommand_RebootSetsRFlag(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 0)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[2], gc.Equals, "-r")
}

func (s *RebootSuite) TestBuildRebootCommand_HaltSetsSFlag(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldShutdown, 0)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[2], gc.Equals, "-s")
}

// const (
// 	rebootBin  = "shutdown.exe"
// 	rebootTime = "15"
// )

// func (s *RebootSuite) rebootCommandParams(c *gc.C) []string {
// 	return []string{
// 		"-f",
// 		"-r",
// 		"-t",
// 		rebootTime,
// 	}
// }

// func (s *RebootSuite) shutdownCommandParams(c *gc.C) []string {
// 	return []string{
// 		"-f",
// 		"-s",
// 		"-t",
// 		rebootTime,
// 	}
// }

// func (s *RebootSuite) TestRebootNoContainers(c *gc.C) {
// 	w, err := NewRebootWaiter(s.st, s.acfg)
// 	c.Assert(err, jc.ErrorIsNil)
// 	expectedRebootParams := s.rebootCommandParams(c)

// 	err = w.ExecuteReboot(params.ShouldReboot)
// 	c.Assert(err, jc.ErrorIsNil)
// 	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
// }

// func (s *RebootSuite) TestShutdownNoContainers(c *gc.C) {
// 	w, err := NewRebootWaiter(s.st, s.acfg)
// 	c.Assert(err, jc.ErrorIsNil)
// 	expectedShutdownParams := s.shutdownCommandParams(c)

// 	err = w.ExecuteReboot(params.ShouldShutdown)
// 	c.Assert(err, jc.ErrorIsNil)
// 	testing.AssertEchoArgs(c, rebootBin, expectedShutdownParams...)
// }
