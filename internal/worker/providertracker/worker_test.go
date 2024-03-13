// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker_test

import (
	"context"
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/worker/providertracker"
	coretesting "github.com/juju/juju/testing"
)

type WorkerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) validConfig(observer providertracker.ConfigObserver) providertracker.Config {
	if observer == nil {
		observer = &testObserver{}
	}
	return providertracker.Config{
		Observer:       observer,
		NewEnvironFunc: newMockEnviron,
		Logger:         loggo.GetLogger("test"),
	}
}

func (s *WorkerSuite) TestValidateObserver(c *gc.C) {
	config := s.validConfig(nil)
	config.Observer = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, "nil Observer not valid")
	})
}

func (s *WorkerSuite) TestValidateNewEnvironFunc(c *gc.C) {
	config := s.validConfig(nil)
	config.NewEnvironFunc = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, "nil NewEnvironFunc not valid")
	})
}

func (s *WorkerSuite) TestValidateLogger(c *gc.C) {
	config := s.validConfig(nil)
	config.Logger = nil
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, "nil Logger not valid")
	})
}

func (s *WorkerSuite) testValidate(c *gc.C, config providertracker.Config, check func(err error)) {
	err := config.Validate()
	check(err)

	worker, err := providertracker.NewWorker(context.Background(), config)
	c.Check(worker, gc.IsNil)
	check(err)
}

func (s *WorkerSuite) TestModelConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			errors.New("no you"),
		},
	}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Check(err, gc.ErrorMatches, "retrieving model config: no you")
		c.Check(worker, gc.IsNil)
		observer.CheckCallNames(c, "ModelConfig")
	})
}

func (s *WorkerSuite) TestModelConfigInvalid(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(observer *testObserver) {
		config := s.validConfig(observer)
		config.NewEnvironFunc = func(context.Context, environs.OpenParams) (environs.Environ, error) {
			return nil, errors.NotValidf("config")
		}
		worker, err := providertracker.NewWorker(context.Background(), config)
		c.Check(err, gc.ErrorMatches,
			`creating environ for model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): config not valid`)
		c.Check(worker, gc.IsNil)
		observer.CheckCallNames(c, "ModelConfig", "CloudSpec")
	})
}

func (s *WorkerSuite) TestModelConfigValid(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "this-particular-name",
		},
	}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, worker)

		gotEnviron := worker.Environ()
		c.Assert(gotEnviron, gc.NotNil)
		c.Check(gotEnviron.Config().Name(), gc.Equals, "this-particular-name")
	})
}

func (s *WorkerSuite) TestCloudSpec(c *gc.C) {
	cloudSpec := environscloudspec.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	fix := &fixture{initialSpec: cloudSpec}
	fix.Run(c, func(observer *testObserver) {
		config := s.validConfig(observer)
		config.NewEnvironFunc = func(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
			c.Assert(args.Cloud, jc.DeepEquals, cloudSpec)
			return nil, errors.NotValidf("cloud spec")
		}
		worker, err := providertracker.NewWorker(context.Background(), config)
		c.Check(err, gc.ErrorMatches,
			`creating environ for model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): cloud spec not valid`)
		c.Check(worker, gc.IsNil)
		observer.CheckCallNames(c, "ModelConfig", "CloudSpec")
	})
}

func (s *WorkerSuite) TestWatchFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, errors.New("grrk splat"),
		},
	}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, worker)

		err = workertest.CheckKilled(c, worker)
		c.Check(err, gc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): watching environ config: grrk splat`)
		observer.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges")
	})
}

func (s *WorkerSuite) TestModelConfigWatchCloses(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, worker)

		observer.CloseModelConfigNotify()
		err = workertest.CheckKilled(c, worker)
		c.Check(err, gc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): environ config watch closed`)
		observer.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "WatchCloudSpecChanges")
	})
}

func (s *WorkerSuite) TestCloudSpecWatchCloses(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, worker)

		observer.CloseCloudSpecNotify()
		err = workertest.CheckKilled(c, worker)
		c.Check(err, gc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): cloud watch closed`)
		observer.CheckCallNames(c, "ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "WatchCloudSpecChanges")
	})
}

func (s *WorkerSuite) TestWatchedModelConfigFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			nil, nil, nil, nil, errors.New("blam ouch"),
		},
	}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, worker)

		observer.SendModelConfigNotify()
		err = workertest.CheckKilled(c, worker)
		c.Check(err, gc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): reading model config: blam ouch`)
	})
}

func (s *WorkerSuite) TestWatchedModelConfigIncompatible(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(observer *testObserver) {
		config := s.validConfig(observer)
		config.NewEnvironFunc = func(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
			env := &mockEnviron{cfg: args.Config}
			env.SetErrors(nil, errors.New("SetConfig is broken"))
			return env, nil
		}
		worker, err := providertracker.NewWorker(context.Background(), config)
		c.Check(err, jc.ErrorIsNil)
		defer workertest.DirtyKill(c, worker)

		observer.SendModelConfigNotify()
		err = workertest.CheckKilled(c, worker)
		c.Check(err, gc.ErrorMatches,
			`model \"testmodel\" \(deadbeef-0bad-400d-8000-4b1d0d06f00d\): updating environ config: SetConfig is broken`)
		observer.CheckCallNames(c,
			"ModelConfig", "CloudSpec", "WatchForModelConfigChanges", "WatchCloudSpecChanges", "ModelConfig")
	})
}

func (s *WorkerSuite) TestWatchedModelConfigUpdates(c *gc.C) {
	fix := &fixture{
		initialConfig: coretesting.Attrs{
			"name": "original-name",
		},
	}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Check(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, worker)

		observer.SetConfig(c, coretesting.Attrs{
			"name": "updated-name",
		})
		gotEnviron := worker.Environ()
		c.Assert(gotEnviron.Config().Name(), gc.Equals, "original-name")

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		observer.SendModelConfigNotify()
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
	})
}

func (s *WorkerSuite) TestWatchedCloudSpecUpdates(c *gc.C) {
	fix := &fixture{
		initialSpec: environscloudspec.CloudSpec{Name: "cloud", Type: "lxd"},
	}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Check(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, worker)

		observer.SetCloudSpec(c, environscloudspec.CloudSpec{Name: "lxd", Type: "lxd", Endpoint: "http://api"})
		gotEnviron := worker.Environ().(*mockEnviron)
		c.Assert(gotEnviron.CloudSpec(), jc.DeepEquals, fix.initialSpec)

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		observer.SendCloudSpecNotify()
		for {
			select {
			case <-attempt:
				ep := gotEnviron.CloudSpec().Endpoint
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

func (s *WorkerSuite) TestWatchedCloudSpecCredentialsUpdates(c *gc.C) {
	original := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "user",
			"password": "secret",
		},
	)
	differentContent := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "user",
			"password": "not-secret-anymore",
		},
	)
	fix := &fixture{
		initialSpec: environscloudspec.CloudSpec{Name: "cloud", Type: "lxd", Credential: &original},
	}
	fix.Run(c, func(observer *testObserver) {
		worker, err := providertracker.NewWorker(context.Background(), s.validConfig(observer))
		c.Check(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, worker)

		observer.SetCloudSpec(c, environscloudspec.CloudSpec{Name: "lxd", Type: "lxd", Credential: &differentContent})
		gotEnviron := worker.Environ().(*mockEnviron)
		c.Assert(gotEnviron.CloudSpec(), jc.DeepEquals, fix.initialSpec)

		timeout := time.After(coretesting.LongWait)
		attempt := time.After(0)
		observer.SendCloudSpecNotify()
		for {
			select {
			case <-attempt:
				ep := gotEnviron.CloudSpec().Credential
				if reflect.DeepEqual(ep, &original) {
					attempt = time.After(coretesting.ShortWait)
					continue
				}
				c.Check(reflect.DeepEqual(ep, &differentContent), jc.IsTrue)
			case <-timeout:
				c.Fatalf("timed out waiting for environ to be updated")
			}
			break
		}
	})
}
