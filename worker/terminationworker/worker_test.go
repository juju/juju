// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker_test

import (
	"os"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/worker"
	"github.com/juju/juju/v3/worker/terminationworker"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&TerminationWorkerSuite{})

type TerminationWorkerSuite struct {
	// c is a channel that will wait for the termination
	// signal, to prevent signals terminating the process.
	c chan os.Signal
}

func (s *TerminationWorkerSuite) TestStartStop(c *gc.C) {
	w := terminationworker.NewWorker()
	w.Kill()
	err := w.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TerminationWorkerSuite) TestSignal(c *gc.C) {
	w := terminationworker.NewWorker()
	proc, err := os.FindProcess(os.Getpid())
	c.Assert(err, jc.ErrorIsNil)
	defer proc.Release()
	err = proc.Signal(terminationworker.TerminationSignal)
	c.Assert(err, jc.ErrorIsNil)
	err = w.Wait()
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
}
