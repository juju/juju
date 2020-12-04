// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
)

const (
	rebootBin  = "shutdown.exe"
	rebootTime = "15"
)

func (s *WinRebootSuite) rebootCommandParams(c *gc.C) []string {
	return []string{
		"-f",
		"-r",
		"-t",
		rebootTime,
	}
}

func (s *WinRebootSuite) shutdownCommandParams(c *gc.C) []string {
	return []string{
		"-f",
		"-s",
		"-t",
		rebootTime,
	}
}

type WinRebootSuite struct {
	jujutesting.BaseSuite
}

func (s *WinRebootSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	testing.PatchExecutableAsEchoArgs(c, s, rebootBin)
}

var _ = gc.Suite(&WinRebootSuite{})

func (s *WinRebootSuite) TestRebootNoContainers(c *gc.C) {
	expectedRebootParams := s.rebootCommandParams(c)
	err := scheduleAction(params.ShouldReboot, 15)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
}

func (s *WinRebootSuite) TestShutdownNoContainers(c *gc.C) {
	expectedShutdownParams := s.shutdownCommandParams(c)
	err := scheduleAction(params.ShouldShutdown, 15)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedShutdownParams...)
}
