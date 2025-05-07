// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"context"
	"errors"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type simpleWorkerSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&simpleWorkerSuite{})

var errTest = errors.New("test error")

func (s *simpleWorkerSuite) TestWait(c *tc.C) {
	doWork := func(context.Context) error {
		return errTest
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), tc.Equals, errTest)
}

func (s *simpleWorkerSuite) TestWaitNil(c *tc.C) {
	doWork := func(context.Context) error {
		return nil
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), tc.Equals, nil)
}

func (s *simpleWorkerSuite) TestKill(c *tc.C) {
	doWork := func(ctx context.Context) error {
		<-ctx.Done()
		return errTest
	}

	w := NewSimpleWorker(doWork)
	w.Kill()
	c.Assert(w.Wait(), tc.Equals, errTest)

	// test we can kill again without a panic
	w.Kill()
}
