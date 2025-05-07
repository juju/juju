// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/common"
)

type cleanupSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&cleanupSuite{})

func (s *cleanupSuite) TestCleansUpOnce(c *tc.C) {
	var w fakeWorker
	cleanup := func() {
		w.stub.AddCall("cleanup")
	}
	w.stub.SetErrors(errors.Errorf("oops"))
	cw := common.NewCleanupWorker(&w, cleanup)
	c.Assert(cw.Wait(), tc.ErrorMatches, "oops")
	w.stub.CheckCallNames(c, "Wait", "cleanup")
	c.Assert(cw.Wait(), jc.ErrorIsNil)
	// Doesn't call cleanup again.
	w.stub.CheckCallNames(c, "Wait", "cleanup", "Wait")
}

func (s *cleanupSuite) TestReport(c *tc.C) {
	var w fakeWorker
	cw := common.NewCleanupWorker(&w, func() {})
	defer workertest.CleanKill(c, cw)

	reporter, ok := cw.(worker.Reporter)
	c.Assert(ok, jc.IsTrue)
	c.Assert(reporter.Report(), jc.DeepEquals, map[string]interface{}{
		"fake": true,
	})
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

func (w *fakeWorker) Report() map[string]interface{} {
	return map[string]interface{}{
		"fake": true,
	}
}
