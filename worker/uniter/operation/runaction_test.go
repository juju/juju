// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type RunActionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunActionSuite{})

func (s *RunActionSuite) TestPrepareErrorBadAction(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunActionSuite) TestPrepareErrorActionNotAvailable(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunActionSuite) TestPrepareErrorOther(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunActionSuite) TestPrepareSuccess(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunActionSuite) TestExecuteLockError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunActionSuite) TestExecuteRunError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunActionSuite) TestExecuteSuccess(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *RunActionSuite) TestCommit(c *gc.C) {
	c.Fatalf("XXX")
}
