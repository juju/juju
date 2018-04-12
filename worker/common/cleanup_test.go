// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/common"
)

type cleanupSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&cleanupSuite{})

func (s *cleanupSuite) TestCleansUpOnce(c *gc.C) {
	var w fakeWorker
	cleanup := func() {
		w.stub.AddCall("cleanup")
	}
	w.stub.SetErrors(errors.Errorf("oops"))
	cw := common.NewCleanupWorker(&w, cleanup)
	c.Assert(cw.Wait(), gc.ErrorMatches, "oops")
	w.stub.CheckCallNames(c, "Wait", "cleanup")
	c.Assert(cw.Wait(), jc.ErrorIsNil)
	// Doesn't call cleanup again.
	w.stub.CheckCallNames(c, "Wait", "cleanup", "Wait")
}

type fakeWorker struct {
	stub testing.Stub
}

func (w *fakeWorker) Kill() {
	w.stub.AddCall("Kill")
}

func (w *fakeWorker) Wait() error {
	w.stub.AddCall("Wait")
	return w.stub.NextErr()
}
