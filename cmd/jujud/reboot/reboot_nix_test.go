// +build !windows

package reboot

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

// // on linux we use the "at" command to schedule a reboot
// var rebootBin = "at"

// var expectedRebootScript = `#!/bin/bash
// sleep 15
// shutdown -r now`

// var expectedShutdownScript = `#!/bin/bash
// sleep 15
// shutdown -h now`

// var lxcLsScript = `#!/bin/bash
// echo juju-machine-1-lxc-0
// `

// var lxcInfoScriptMissbehave = `#!/bin/bash
// echo '
// Name:           juju-machine-1-lxc-0
// State:          RUNNING
// PID:            13955
// IP:             192.168.200.85
// CPU use:        186.37 seconds
// BlkIO use:      175.29 MiB
// Memory use:     202.45 MiB
// Link:           vethXUAOWB
//  TX bytes:      516.81 KiB
//  RX bytes:      12.31 MiB
//  Total bytes:   12.82 MiB
// '
// `

// var lxcInfoScript = `#!/bin/bash
// LINE_COUNT=$(wc -l "$TEMP/empty-lxc-response" 2>/dev/null | awk '{print $1}')
// RAN=${LINE_COUNT:-0}

// if [ "$RAN" -ge 3 ]
// then
//     echo ""
// else
//     echo '
// Name:           juju-machine-1-lxc-0
// State:          RUNNING
// PID:            13955
// IP:             192.168.200.85
// CPU use:        186.37 seconds
// BlkIO use:      175.29 MiB
// Memory use:     202.45 MiB
// Link:           vethXUAOWB
//  TX bytes:      516.81 KiB
//  RX bytes:      12.31 MiB
//  Total bytes:   12.82 MiB
// '
//     echo 1 >> "$TEMP/empty-lxc-response"
// fi
// `

func (s *RebootSuite) TestBuildRebootCommand_ShouldDoNothing(c *gc.C) {
	cmd, args := buildRebootCommand(params.ShouldDoNothing, 0)
	c.Check(cmd, gc.Equals, "")
	c.Check(args, gc.HasLen, 0)
}

func (s *RebootSuite) TestBuildRebootCommand_IsShutdownCmd(c *gc.C) {
	cmd, _ := buildRebootCommand(params.ShouldReboot, 0)
	c.Check(cmd, gc.Equals, "shutdown")
}

func (s *RebootSuite) TestBuildRebootCommand_SetsDelay(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 1*time.Minute)
	c.Assert(args, gc.HasLen, 2)
	c.Check(args[1], gc.Equals, "+1")
}

func (s *RebootSuite) TestBuildRebootCommand_DelayToCeilingSecond(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 2*time.Second)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[1], gc.Equals, "+1")
}

func (s *RebootSuite) TestBuildRebootCommand_RebootSetsRFlag(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 0)
	c.Assert(len(args), gc.Not(jc.LessThan), 1)
	c.Check(args[0], gc.Equals, "-r")
}

func (s *RebootSuite) TestBuildRebootCommand_HaltSetsHFlag(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldShutdown, 0)
	c.Assert(len(args), gc.Not(jc.LessThan), 1)
	c.Check(args[0], gc.Equals, "-h")
}

// func (s *RebootSuite) TestExecuteReboot_WithContainers(c *gc.C) {
// 	// testing.PatchExecutable(c, s, "lxc-ls", lxcLsScript)
// 	// testing.PatchExecutable(c, s, "lxc-info", lxcInfoScript)
// 	expectedRebootParams := s.rebootCommandParams(c)

// 	newContainerManager := newContainerManagerFn(
// 		&fakeContainerManager{
// 			listContainers: func([]instance.Instance, error) {
// 				return nil, nil
// 			},
// 		},
// 	)

// 	w, err := newRebootWaiter(exec, newContainerManager, s.st, s.acfg, clock.WallClock, 5*time.Second)
// 	c.Assert(err, jc.ErrorIsNil)

// 	err = w.ExecuteReboot(params.ShouldReboot)
// 	c.Assert(err, jc.ErrorIsNil)
// 	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
// 	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
// }

// func (s *RebootSuite) TestRebootWithMissbehavingContainers(c *gc.C) {
// 	testing.PatchExecutable(c, s, "lxc-ls", lxcLsScript)
// 	testing.PatchExecutable(c, s, "lxc-info", lxcInfoScriptMissbehave)

// 	s.PatchValue(Timeout, time.Duration(1*time.Second))
// 	w, err := NewRebootWaiter(s.st, s.acfg)
// 	c.Assert(err, jc.ErrorIsNil)

// 	err = w.ExecuteReboot(params.ShouldReboot)
// 	c.Assert(err, gc.ErrorMatches, "Timeout reached waiting for containers to shutdown")
// }

// func (s *RebootSuite) TestRebootNoContainers(c *gc.C) {
// 	w, err := NewRebootWaiter(s.st, s.acfg)
// 	c.Assert(err, jc.ErrorIsNil)
// 	expectedRebootParams := s.rebootCommandParams(c)

// 	err = w.ExecuteReboot(params.ShouldReboot)
// 	c.Assert(err, jc.ErrorIsNil)
// 	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
// 	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
// }

// func (s *RebootSuite) TestShutdownNoContainers(c *gc.C) {
// 	w, err := NewRebootWaiter(s.st, s.acfg)
// 	c.Assert(err, jc.ErrorIsNil)
// 	expectedShutdownParams := s.shutdownCommandParams(c)

// 	err = w.ExecuteReboot(params.ShouldShutdown)
// 	c.Assert(err, jc.ErrorIsNil)
// 	testing.AssertEchoArgs(c, rebootBin, expectedShutdownParams...)
// 	ft.File{s.rebootScriptName, expectedShutdownScript, 0755}.Check(c, s.tmpDir)
// }
