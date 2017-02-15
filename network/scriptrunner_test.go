// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"os"
	"runtime"
	"time"

	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
)

type ScriptRunnerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ScriptRunnerSuite{})

func (s *ScriptRunnerSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("skipping ScriptRunnerSuite tests on windows")
	}
	s.IsolationSuite.SetUpSuite(c)
}

func (*ScriptRunnerSuite) TestScriptRunnerFails(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand("exit 1", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 1)
}

func (*ScriptRunnerSuite) TestScriptRunnerSucceeds(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand("exit 0", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 0)
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStdout(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand("echo -n 42", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "42")
	c.Check(string(result.Stderr), gc.Equals, "")
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStderr(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand(">&2 echo -n 3.141", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "3.141")
}

func (*ScriptRunnerSuite) TestScriptRunnerTimeout(c *gc.C) {
	result, err := network.RunCommand("sleep 6", os.Environ(), clock.WallClock, 500*time.Microsecond)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, true)
	c.Assert(result.Code, gc.Equals, 0)
}
