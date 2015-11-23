// Copyright 2012, 2013 Canonical Ltd.
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

type TrackerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TrackerSuite{})

func (s *TrackerSuite) TestValidateObserver(c *gc.C) {
	config := environ.Config{}
	check := func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil Observer not valid")
	}

	err := config.Validate()
	check(err)

	tracker, err := environ.NewTracker(config)
	c.Check(tracker, gc.IsNil)
	check(err)
}

func (s *TrackerSuite) TestEnvironConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			errors.New("no yuo"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Check(err, gc.ErrorMatches, "cannot read environ config: no yuo")
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "EnvironConfig")
	})

}

func (s *TrackerSuite) TestEnvironConfigInvalid(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"type": "unknown",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Check(err, gc.ErrorMatches, `cannot create environ: no registered provider for "unknown"`)
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "EnvironConfig")
	})

}

func (s *TrackerSuite) TestEnvironConfigValid(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "this-particular-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotEnviron := tracker.Environ()
		c.Assert(gotEnviron, gc.NotNil)
		c.Check(gotEnviron.Config().Name(), gc.Equals, "this-particular-name")
	})
}

func (s *TrackerSuite) TestWatchFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, errors.New("grrk splat"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot watch environ config: grrk splat")
		context.CheckCallNames(c, "EnvironConfig", "WatchForEnvironConfigChanges")
	})
}

func (s *TrackerSuite) TestWatchCloses(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.CloseNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "environ config watch closed")
		context.CheckCallNames(c, "EnvironConfig", "WatchForEnvironConfigChanges")
	})
}

func (s *TrackerSuite) TestWatchedEnvironConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, errors.New("blam ouch"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.SendNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot read environ config: blam ouch")
		context.CheckCallNames(c, "EnvironConfig", "WatchForEnvironConfigChanges", "EnvironConfig")
	})
}

func (s *TrackerSuite) TestWatchedEnvironConfigIncompatible(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"broken": "SetConfig",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.SendNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot update environ config: dummy.SetConfig is broken")
		context.CheckCallNames(c, "EnvironConfig", "WatchForEnvironConfigChanges", "EnvironConfig")
	})
}

func (s *TrackerSuite) TestWatchedEnvironConfigUpdates(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "original-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		context.SetConfig(c, coretesting.Attrs{
			"name": "updated-name",
		})
		gotEnviron := tracker.Environ()
		c.Assert(gotEnviron.Config().Name(), gc.Equals, "original-name")

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		context.SendNotify()
		for {
			select {
			case <-attempt:
				name := gotEnviron.Config().Name()
				if name == "original-name" {
					attempt = time.After(coretesting.ShortWait)
					continue
				}
				c.Check(name, gc.Equals, "updated-name")
			case <-timeout:
				c.Fatalf("timed out waiting for environ to be updated")
			}
			break
		}
		context.CheckCallNames(c, "EnvironConfig", "WatchForEnvironConfigChanges", "EnvironConfig")
	})
}
