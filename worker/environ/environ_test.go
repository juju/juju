// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
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
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil Observer not valid")
	})
}

func (s *TrackerSuite) TestValidateNewEnvironFunc(c *gc.C) {
	config := environ.Config{
		Observer: &runContext{},
	}
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil NewEnvironFunc not valid")
	})
}

func (s *TrackerSuite) testValidate(c *gc.C, config environ.Config, check func(err error)) {
	err := config.Validate()
	check(err)

	tracker, err := environ.NewTracker(config)
	c.Check(tracker, gc.IsNil)
	check(err)
}

func (s *TrackerSuite) TestModelConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			errors.New("no yuo"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer:       context,
			NewEnvironFunc: newMockEnviron,
		})
		c.Check(err, gc.ErrorMatches, "cannot create environ: no yuo")
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "ModelConfig")
	})
}

func (s *TrackerSuite) TestModelConfigInvalid(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
			NewEnvironFunc: func(environs.OpenParams) (environs.Environ, error) {
				return nil, errors.NotValidf("config")
			},
		})
		c.Check(err, gc.ErrorMatches, `cannot create environ: config not valid`)
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "ModelConfig", "CloudSpec")
	})
}

func (s *TrackerSuite) TestModelConfigValid(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "this-particular-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer:       context,
			NewEnvironFunc: newMockEnviron,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotEnviron := tracker.Environ()
		c.Assert(gotEnviron, gc.NotNil)
		c.Check(gotEnviron.Config().Name(), gc.Equals, "this-particular-name")
	})
}

func (s *TrackerSuite) TestCloudSpec(c *gc.C) {
	cloudSpec := environs.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	fix := &fixture{cloud: cloudSpec}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
			NewEnvironFunc: func(args environs.OpenParams) (environs.Environ, error) {
				c.Assert(args.Cloud, jc.DeepEquals, cloudSpec)
				return nil, errors.NotValidf("cloud spec")
			},
		})
		c.Check(err, gc.ErrorMatches, `cannot create environ: cloud spec not valid`)
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "ModelConfig", "CloudSpec")
	})
}

func (s *TrackerSuite) TestWatchFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, errors.New("grrk splat"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer:       context,
			NewEnvironFunc: newMockEnviron,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot watch environ config: grrk splat")
		context.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges")
	})
}

func (s *TrackerSuite) TestWatchCloses(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer:       context,
			NewEnvironFunc: newMockEnviron,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.CloseModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "environ config watch closed")
		context.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, nil, errors.New("blam ouch"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer:       context,
			NewEnvironFunc: newMockEnviron,
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.SendModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot read environ config: blam ouch")
		context.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "ModelConfig")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigIncompatible(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer: context,
			NewEnvironFunc: func(environs.OpenParams) (environs.Environ, error) {
				env := &mockEnviron{}
				env.SetErrors(errors.New("SetConfig is broken"))
				return env, nil
			},
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.SendModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot update environ config: SetConfig is broken")
		context.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "ModelConfig")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigUpdates(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "original-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := environ.NewTracker(environ.Config{
			Observer:       context,
			NewEnvironFunc: newMockEnviron,
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
		context.SendModelConfigNotify()
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
		context.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "ModelConfig")
	})
}
