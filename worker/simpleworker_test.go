// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type simpleWorkerSuite struct {
	testbase.LoggingSuite
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

func (s *simpleWorkerSuite) TestKill(c *gc.C) {
	doWork := func(stopCh <-chan struct{}) error {
		<-stopCh
		return nil
	}

	w := NewSimpleWorker(doWork)
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, nil)
}
