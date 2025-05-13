// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker_test

import (
	"os"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/terminationworker"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

var _ = tc.Suite(&TerminationWorkerSuite{})

type TerminationWorkerSuite struct{}

func (s *TerminationWorkerSuite) TestStartStop(c *tc.C) {
	w := terminationworker.NewWorker()
	w.Kill()
	err := w.Wait()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TerminationWorkerSuite) TestSignal(c *tc.C) {
	w := terminationworker.NewWorker()
	proc, err := os.FindProcess(os.Getpid())
	c.Assert(err, tc.ErrorIsNil)
	defer proc.Release()
	err = proc.Signal(terminationworker.TerminationSignal)
	c.Assert(err, tc.ErrorIsNil)
	err = w.Wait()
	c.Assert(err, tc.Equals, worker.ErrTerminateAgent)
}
