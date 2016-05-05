// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"errors"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
)

type RebootSuite struct{}

var _ = gc.Suite(&RebootSuite{})

func (s *RebootSuite) TestExecuteReboot_ShouldDoNothingReturns(c *gc.C) {
	var execCalled bool
	exec := func(string, ...string) (string, error) { execCalled = true; return "", nil }
	err := ExecuteReboot(exec, 0, params.ShouldDoNothing)
	c.Check(err, jc.ErrorIsNil)
	// We should return immediately without calling exec.
	c.Check(execCalled, gc.Equals, false)
}

func (s *RebootSuite) TestExecuteReboot_ExecuteBuiltShutdownCmd(c *gc.C) {
	var execCalled bool
	exec := func(string, ...string) (string, error) { execCalled = true; return "", nil }
	err := ExecuteReboot(exec, 0, params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(execCalled, gc.Equals, true)
}

func (s *RebootSuite) TestExecuteReboot_ReturnExecError(c *gc.C) {
	var execCalled bool
	exec := func(string, ...string) (string, error) {
		execCalled = true
		return "", errors.New("foo")
	}
	err := ExecuteReboot(exec, 0, params.ShouldReboot)
	c.Assert(err, gc.NotNil)
	c.Check(execCalled, gc.Equals, true)
	c.Check(err, gc.ErrorMatches, "cannot schedule reboot: foo")
}
