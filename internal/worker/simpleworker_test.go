// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"context"
	"errors"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type simpleWorkerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&simpleWorkerSuite{})

var errTest = errors.New("test error")

func (s *simpleWorkerSuite) TestWait(c *gc.C) {
	doWork := func(context.Context) error {
		return errTest
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), gc.Equals, errTest)
}

func (s *simpleWorkerSuite) TestWaitNil(c *gc.C) {
	doWork := func(context.Context) error {
		return nil
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), gc.Equals, nil)
}

func (s *simpleWorkerSuite) TestKill(c *gc.C) {
	doWork := func(ctx context.Context) error {
		<-ctx.Done()
		return errTest
	}

	w := NewSimpleWorker(doWork)
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, errTest)

	// test we can kill again without a panic
	w.Kill()
}
