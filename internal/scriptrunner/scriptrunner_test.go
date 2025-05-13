// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptrunner_test

import (
	"os"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/scriptrunner"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}

type ScriptRunnerSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ScriptRunnerSuite{})

func (s *ScriptRunnerSuite) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func (*ScriptRunnerSuite) TestScriptRunnerFails(c *tc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand("exit 1", os.Environ(), clock, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Code, tc.Equals, 1)
}

func (*ScriptRunnerSuite) TestScriptRunnerSucceeds(c *tc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand("exit 0", os.Environ(), clock, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Code, tc.Equals, 0)
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStdout(c *tc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand("echo -n 42", os.Environ(), clock, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Code, tc.Equals, 0)
	c.Check(string(result.Stdout), tc.Equals, "42")
	c.Check(string(result.Stderr), tc.Equals, "")
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStderr(c *tc.C) {
	clock := testclock.NewClock(coretesting.ZeroTime())
	result, err := scriptrunner.RunCommand(">&2 echo -n 3.141", os.Environ(), clock, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Code, tc.Equals, 0)
	c.Check(string(result.Stdout), tc.Equals, "")
	c.Check(string(result.Stderr), tc.Equals, "3.141")
}

func (*ScriptRunnerSuite) TestScriptRunnerTimeout(c *tc.C) {
	_, err := scriptrunner.RunCommand("sleep 6", os.Environ(), clock.WallClock, 500*time.Microsecond)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, "command cancelled")
}
