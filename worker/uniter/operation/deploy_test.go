// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type DeploySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DeploySuite{})

func (s *DeploySuite) TestPrepareAlreadyDone(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestPrepareArchiveInfoError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestPrepareStageError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestPrepareSetCharmError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestPrepareSuccess(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestExecuteError(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestExecuteSuccess(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestCommitQueueInstallHook(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestCommitQueueUpgradeHook(c *gc.C) {
	c.Fatalf("XXX")
}

func (s *DeploySuite) TestCommitInterruptedHook(c *gc.C) {
	c.Fatalf("XXX")
}
