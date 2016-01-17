// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/environ"
	"github.com/juju/juju/worker/workertest"
)

type WaitSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&WaitSuite{})

func (s *WaitSuite) TestWaitAborted(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		abort := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			env, err := environ.WaitForEnviron(context.watcher, nil, abort)
			c.Check(env, gc.IsNil)
			c.Check(err, gc.Equals, environ.ErrWaitAborted)
		}()

		close(abort)
		select {
		case <-done:
		case <-time.After(coretesting.LongWait):
			c.Errorf("timed out waiting for abort")
		}
		workertest.CheckAlive(c, context.watcher)
	})
}

func (s *WaitSuite) TestWatchClosed(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		abort := make(chan struct{})
		defer close(abort)

		done := make(chan struct{})
		go func() {
			defer close(done)
			env, err := environ.WaitForEnviron(context.watcher, nil, abort)
			c.Check(env, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "environ config watch closed")
		}()

		context.CloseNotify()
		select {
		case <-done:
		case <-time.After(coretesting.LongWait):
			c.Errorf("timed out waiting for failure")
		}
		workertest.CheckAlive(c, context.watcher)
	})
}

func (s *WaitSuite) TestConfigError(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			errors.New("biff zonk"),
		},
	}
	fix.Run(c, func(context *runContext) {
		abort := make(chan struct{})
		defer close(abort)

		done := make(chan struct{})
		go func() {
			defer close(done)
			env, err := environ.WaitForEnviron(context.watcher, context, abort)
			c.Check(env, gc.IsNil)
			c.Check(err, gc.ErrorMatches, "cannot read environ config: biff zonk")
		}()

		context.SendNotify()
		select {
		case <-done:
		case <-time.After(coretesting.LongWait):
			c.Errorf("timed out waiting for failure")
		}
		workertest.CheckAlive(c, context.watcher)
	})
}

func (s *WaitSuite) TestIgnoresBadConfig(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"type": "unknown",
		},
	}
	fix.Run(c, func(context *runContext) {
		abort := make(chan struct{})
		defer close(abort)

		done := make(chan struct{})
		go func() {
			defer close(done)
			env, err := environ.WaitForEnviron(context.watcher, context, abort)
			if c.Check(err, jc.ErrorIsNil) {
				c.Check(env.Config().Name(), gc.Equals, "expected-name")
			}
		}()

		context.SendNotify()
		select {
		case <-time.After(coretesting.ShortWait):
		case <-done:
			c.Errorf("completed unexpectedly")
		}

		context.SetConfig(c, coretesting.Attrs{
			"name": "expected-name",
		})
		context.SendNotify()
		select {
		case <-done:
		case <-time.After(coretesting.LongWait):
			c.Errorf("timed out waiting for success")
		}
		workertest.CheckAlive(c, context.watcher)
	})
}
