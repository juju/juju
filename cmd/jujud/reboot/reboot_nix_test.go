// +build !windows

package reboot_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/reboot"
)

// on linux we use the "at" command to schedule a reboot
var rebootBin = "at"

var expectedRebootScript = `#!/bin/bash
sleep 15
shutdown -r now`

var expectedShutdownScript = `#!/bin/bash
sleep 15
shutdown -h now`

var lxcLsScript = `#!/bin/bash
echo juju-machine-1-lxc-0
`

var lxcInfoScriptMissbehave = `#!/bin/bash
echo '
Name:           juju-machine-1-lxc-0
State:          RUNNING
PID:            13955
IP:             192.168.200.85
CPU use:        186.37 seconds
BlkIO use:      175.29 MiB
Memory use:     202.45 MiB
Link:           vethXUAOWB
 TX bytes:      516.81 KiB
 RX bytes:      12.31 MiB
 Total bytes:   12.82 MiB
'
`

var lxcInfoScript = `#!/bin/bash
LINE_COUNT=$(wc -l "$TEMP/empty-lxc-response" 2>/dev/null | awk '{print $1}')
RAN=${LINE_COUNT:-0}

if [ "$RAN" -ge 3 ]
then
    echo ""
else
    echo '
Name:           juju-machine-1-lxc-0
State:          RUNNING
PID:            13955
IP:             192.168.200.85
CPU use:        186.37 seconds
BlkIO use:      175.29 MiB
Memory use:     202.45 MiB
Link:           vethXUAOWB
 TX bytes:      516.81 KiB
 RX bytes:      12.31 MiB
 Total bytes:   12.82 MiB
'
    echo 1 >> "$TEMP/empty-lxc-response"
fi
`

func (s *RebootSuite) rebootCommandParams(c *gc.C) []string {
	return []string{
		"-f",
		s.rebootScript(c),
		"now",
	}
}

func (s *RebootSuite) shutdownCommandParams(c *gc.C) []string {
	return []string{
		"-f",
		s.rebootScript(c),
		"now",
	}
}

func (s *RebootSuite) TestRebootWithContainers(c *gc.C) {
	testing.PatchExecutable(c, s, "lxc-ls", lxcLsScript)
	testing.PatchExecutable(c, s, "lxc-info", lxcInfoScript)
	expectedRebootParams := s.rebootCommandParams(c)

	// Timeout after 5 seconds
	s.PatchValue(reboot.Timeout, time.Duration(5*time.Second))
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, jc.ErrorIsNil)

	err = w.ExecuteReboot(params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
}

func (s *RebootSuite) TestRebootWithMissbehavingContainers(c *gc.C) {
	testing.PatchExecutable(c, s, "lxc-ls", lxcLsScript)
	testing.PatchExecutable(c, s, "lxc-info", lxcInfoScriptMissbehave)
	expectedRebootParams := s.rebootCommandParams(c)

	// Timeout after 5 seconds
	s.PatchValue(reboot.Timeout, time.Duration(5*time.Second))
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, jc.ErrorIsNil)

	err = w.ExecuteReboot(params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
}

func (s *RebootSuite) TestRebootNoContainers(c *gc.C) {
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, jc.ErrorIsNil)
	expectedRebootParams := s.rebootCommandParams(c)

	err = w.ExecuteReboot(params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
}

func (s *RebootSuite) TestShutdownNoContainers(c *gc.C) {
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, jc.ErrorIsNil)
	expectedShutdownParams := s.shutdownCommandParams(c)

	err = w.ExecuteReboot(params.ShouldShutdown)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedShutdownParams...)
	ft.File{s.rebootScriptName, expectedShutdownScript, 0755}.Check(c, s.tmpDir)
}
