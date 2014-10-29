// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker_test

import (
	"os"
	"os/signal"
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/terminationworker"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&TerminationWorkerSuite{})

type TerminationWorkerSuite struct {
	testing.BaseSuite
	// c is a channel that will wait for the termination
	// signal, to prevent signals terminating the process.
	c chan os.Signal
}

func (s *TerminationWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.c = make(chan os.Signal, 1)
	signal.Notify(s.c, terminationworker.TerminationSignal)
}

func (s *TerminationWorkerSuite) TearDownTest(c *gc.C) {
	close(s.c)
	signal.Stop(s.c)
	s.BaseSuite.TearDownTest(c)
}

func (s *TerminationWorkerSuite) TestStartStop(c *gc.C) {
	w := terminationworker.NewWorker()
	w.Kill()
	err := w.Wait()
	c.Assert(err, gc.IsNil)
}

func (s *TerminationWorkerSuite) TestSignal(c *gc.C) {
	w := terminationworker.NewWorker()
	proc, err := os.FindProcess(os.Getpid())
	c.Assert(err, gc.IsNil)
	defer proc.Release()
	err = proc.Signal(terminationworker.TerminationSignal)
	c.Assert(err, gc.IsNil)
	err = w.Wait()
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
}
