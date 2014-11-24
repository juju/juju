// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type RunCommandsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunCommandsSuite{})

func (s *RunCommandsSuite) TestPrepareError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunCommandsSuite) TestPrepareSuccess(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunCommandsSuite) TestExecuteLockError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunCommandsSuite) TestExecuteRequeueRebootError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunCommandsSuite) TestExecuteRebootError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunCommandsSuite) TestExecuteErrorOther(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunCommandsSuite) TestCommit(c *gc.C) {
	c.Fatalf("XXX")
}
