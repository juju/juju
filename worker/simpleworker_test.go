// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type simpleWorkerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&simpleWorkerSuite{})

var testError = errors.New("test error")

func (s *simpleWorkerSuite) TestWait(c *gc.C) {
	doWork := func(_ <-chan struct{}) error {
		return testError
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), gc.Equals, testError)
}

func (s *simpleWorkerSuite) TestWaitNil(c *gc.C) {
	doWork := func(_ <-chan struct{}) error {
		return nil
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), gc.Equals, nil)
}

func (s *simpleWorkerSuite) TestKill(c *gc.C) {
	doWork := func(stopCh <-chan struct{}) error {
		<-stopCh
		return testError
	}

	w := NewSimpleWorker(doWork)
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, testError)

	// test we can kill again without a panic
	w.Kill()
}
