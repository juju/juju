// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/caasbroker"
	coretesting "github.com/juju/juju/testing"
)

type TrackerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TrackerSuite{})

func (s *TrackerSuite) validConfig() caasbroker.Config {
	return caasbroker.Config{
		ConfigAPI: &runContext{},
		NewContainerBrokerFunc: func(context.Context, environs.OpenParams) (caas.Broker, error) {
			return nil, errors.NotImplementedf("test func")
		},
		Logger: loggo.GetLogger("test"),
	}
}

func (s *TrackerSuite) TestValidateObserver(c *gc.C) {
	config := s.validConfig()
	config.ConfigAPI = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, "nil ConfigAPI not valid")
	})
}

func (s *TrackerSuite) TestValidateNewBrokerFunc(c *gc.C) {
	config := s.validConfig()
	config.NewContainerBrokerFunc = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, "nil NewContainerBrokerFunc not valid")
	})
}

func (s *TrackerSuite) TestValidateLogger(c *gc.C) {
	config := s.validConfig()
	config.Logger = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, "nil Logger not valid")
	})
}

func (s *TrackerSuite) testValidate(c *gc.C, config caasbroker.Config, check func(err error)) {
	err := config.Validate()
	check(err)

	tracker, err := caasbroker.NewTracker(config)
	c.Check(tracker, gc.IsNil)
	check(err)
}

func (s *TrackerSuite) TestCloudSpecFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			errors.New("no you"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Check(err, gc.ErrorMatches, "cannot get cloud information: no you")
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec")
	})
}

func (s *TrackerSuite) validFixture() *fixture {
	cloudSpec := environscloudspec.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	cfg := coretesting.FakeConfig()
	cfg["type"] = "kubernetes"
	cfg["uuid"] = uuid.MustNewUUID().String()
	return &fixture{initialSpec: cloudSpec, initialConfig: cfg}
}

func (s *TrackerSuite) TestSuccess(c *gc.C) {
	fix := s.validFixture()
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotBroker := tracker.Broker()
		c.Assert(gotBroker, gc.NotNil)
	})
}

func (s *TrackerSuite) TestInitialise(c *gc.C) {
	fix := s.validFixture()
	fix.Run(c, func(runContext *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI: runContext,
			NewContainerBrokerFunc: func(_ context.Context, args environs.OpenParams) (caas.Broker, error) {
				c.Assert(args.Cloud, jc.DeepEquals, fix.initialSpec)
				c.Assert(args.Config.Name(), jc.DeepEquals, "testmodel")
				return nil, errors.NotValidf("cloud spec")
			},
			Logger: loggo.GetLogger("test"),
		})
		c.Check(err, gc.ErrorMatches, `cannot create caas broker: cloud spec not valid`)
		c.Check(tracker, gc.IsNil)
		runContext.CheckCallNames(c, "CloudSpec", "ModelConfig", "ControllerConfig")
	})
}

func (s *TrackerSuite) TestModelConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil,
			errors.New("no you"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Check(err, gc.ErrorMatches, "no you")
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec", "ModelConfig")
	})
}

func (s *TrackerSuite) TestModelConfigInvalid(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(runContext *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI: runContext,
			NewContainerBrokerFunc: func(context.Context, environs.OpenParams) (caas.Broker, error) {
				return nil, errors.NotValidf("config")
			},
			Logger: loggo.GetLogger("test"),
		})
		c.Check(err, gc.ErrorMatches, `cannot create caas broker: config not valid`)
		c.Check(tracker, gc.IsNil)
		runContext.CheckCallNames(c, "CloudSpec", "ModelConfig", "ControllerConfig")
	})
}

func (s *TrackerSuite) TestModelConfigValid(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "this-particular-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotBroker := tracker.Broker()
		c.Assert(gotBroker, gc.NotNil)
		c.Check(gotBroker.Config().Name(), gc.Equals, "this-particular-name")
	})
}

func (s *TrackerSuite) TestCloudSpecInvalid(c *gc.C) {
	cloudSpec := environscloudspec.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	fix := &fixture{initialSpec: cloudSpec}
	fix.Run(c, func(runContext *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI: runContext,
			NewContainerBrokerFunc: func(_ context.Context, args environs.OpenParams) (caas.Broker, error) {
				c.Assert(args.Cloud, jc.DeepEquals, cloudSpec)
				return nil, errors.NotValidf("cloud spec")
			},
			Logger: loggo.GetLogger("test"),
		})
		c.Check(err, gc.ErrorMatches, `cannot create caas broker: cloud spec not valid`)
		c.Check(tracker, gc.IsNil)
		runContext.CheckCallNames(c, "CloudSpec", "ModelConfig", "ControllerConfig")
	})
}

func (s *TrackerSuite) TestWatchFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, nil, errors.New("grrk splat"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot watch model config: grrk splat")
		context.CheckCallNames(c, "CloudSpec", "ModelConfig", "ControllerConfig", "WatchForModelConfigChanges")
	})
}

func (s *TrackerSuite) TestModelConfigWatchCloses(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.CloseModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "model config watch closed")
		context.CheckCallNames(c, "CloudSpec", "ModelConfig", "ControllerConfig", "WatchForModelConfigChanges", "WatchCloudSpecChanges")
	})
}

func (s *TrackerSuite) TestCloudSpecWatchCloses(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.CloseCloudSpecNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cloud watch closed")
		context.CheckCallNames(c, "CloudSpec", "ModelConfig", "ControllerConfig", "WatchForModelConfigChanges", "WatchCloudSpecChanges")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, nil, nil, nil, errors.New("blam ouch"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		context.SendModelConfigNotify()
		context.SendCloudSpecNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot read model config: blam ouch")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigIncompatible(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(runContext *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI: runContext,
			NewContainerBrokerFunc: func(context.Context, environs.OpenParams) (caas.Broker, error) {
				broker := &mockBroker{}
				broker.SetErrors(errors.New("SetConfig is broken"))
				return broker, nil
			},
			Logger: loggo.GetLogger("test"),
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, tracker)

		runContext.SendModelConfigNotify()
		err = workertest.CheckKilled(c, tracker)
		c.Check(err, gc.ErrorMatches, "cannot update model config: SetConfig is broken")
		runContext.CheckCallNames(c, "CloudSpec", "ModelConfig", "ControllerConfig", "WatchForModelConfigChanges", "WatchCloudSpecChanges", "ModelConfig")
	})
}

func (s *TrackerSuite) TestWatchedModelConfigUpdates(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "original-name",
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		context.SetConfig(c, coretesting.Attrs{
			"name": "updated-name",
		})
		gotBroker := tracker.Broker()
		c.Assert(gotBroker.Config().Name(), gc.Equals, "original-name")

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		context.SendModelConfigNotify()
		for {
			select {
			case <-attempt:
				name := gotBroker.Config().Name()
				if name == "original-name" {
					attempt = time.After(coretesting.ShortWait)
					continue
				}
				c.Check(name, gc.Equals, "updated-name")
			case <-timeout:
				c.Fatalf("timed out waiting for broker to be updated")
			}
			break
		}
	})
}

func (s *TrackerSuite) TestWatchedCloudSpecUpdates(c *gc.C) {
	fix := &fixture{
		initialSpec: environscloudspec.CloudSpec{Name: "cloud", Type: "lxd"},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
			Logger:                 loggo.GetLogger("test"),
		})
		c.Check(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		context.SetCloudSpec(c, environscloudspec.CloudSpec{Name: "lxd", Type: "lxd", Endpoint: "http://api"})
		gotBroker := tracker.Broker().(*mockBroker)
		c.Assert(gotBroker.CloudSpec(), jc.DeepEquals, fix.initialSpec)

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		context.SendCloudSpecNotify()
		for {
			select {
			case <-attempt:
				ep := gotBroker.CloudSpec().Endpoint
				if ep == "" {
					attempt = time.After(coretesting.ShortWait)
					continue
				}
				c.Check(ep, gc.Equals, "http://api")
			case <-timeout:
				c.Fatalf("timed out waiting for environ to be updated")
			}
			break
		}
	})
}
