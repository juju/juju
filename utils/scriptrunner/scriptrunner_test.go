// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptrunner_test

import (
	"os"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/scriptrunner"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type ScriptRunnerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ScriptRunnerSuite{})

func (s *ScriptRunnerSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func (*ScriptRunnerSuite) TestScriptRunnerFails(c *gc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand("exit 1", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Code, gc.Equals, 1)
}

func (*ScriptRunnerSuite) TestScriptRunnerSucceeds(c *gc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand("exit 0", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Code, gc.Equals, 0)
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStdout(c *gc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand("echo -n 42", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "42")
	c.Check(string(result.Stderr), gc.Equals, "")
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStderr(c *gc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand(">&2 echo -n 3.141", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "3.141")
}

func (*ScriptRunnerSuite) TestScriptRunnerTimeout(c *gc.C) {
	_, err := scriptrunner.RunCommand("sleep 6", os.Environ(), clock.WallClock, 500*time.Microsecond)
	c.Assert(err, gc.ErrorMatches, `running command "sleep 6": command cancelled`)
}
