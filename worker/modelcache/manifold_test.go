// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcache_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/modelcache"
	workerstate "github.com/juju/juju/worker/state"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config modelcache.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = modelcache.ManifoldConfig{
		StateName:            "state",
		CentralHubName:       "central-hub",
		MultiwatcherName:     "multiwatcher",
		InitializedGateName:  "initialized-gate",
		Logger:               loggo.GetLogger("test"),
		PrometheusRegisterer: noopRegisterer{},
		NewWorker: func(modelcache.Config) (worker.Worker, error) {
			return nil, errors.New("boom")
		},
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return modelcache.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold().Inputs, jc.DeepEquals, []string{
		"state", "central-hub", "multiwatcher", "initialized-gate"})
}

func (s *ManifoldSuite) TestConfigValidation(c *gc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingStateName(c *gc.C) {
	s.config.StateName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing StateName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingCentralHubName(c *gc.C) {
	s.config.CentralHubName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing CentralHubName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingInitializedGateName(c *gc.C) {
	s.config.InitializedGateName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing InitializedGateName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingMultiwatcherName(c *gc.C) {
	s.config.MultiwatcherName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing MultiwatcherName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingPrometheusRegisterer(c *gc.C) {
	s.config.PrometheusRegisterer = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing PrometheusRegisterer not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing Logger not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing NewWorker func not valid")
}

func (s *ManifoldSuite) TestManifoldCallsValidate(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{})
	s.config.Logger = nil
	w, err := s.manifold().Start(context)
	c.Check(w, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Logger not valid`)
}

func (s *ManifoldSuite) TestStateMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            dependency.ErrMissing,
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestCentralHubMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            &fakeStateTracker{},
		"central-hub":      dependency.ErrMissing,
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestMultiwatcherMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            &fakeStateTracker{},
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     dependency.ErrMissing,
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestInitializedGateMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            &fakeStateTracker{},
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": dependency.ErrMissing,
	})

	w, err := s.manifold().Start(context)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *gc.C) {
	var config modelcache.Config
	s.config.NewWorker = func(c modelcache.Config) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            tracker,
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	w, err := s.manifold().Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.NotNil)

	c.Check(config.Validate(), jc.ErrorIsNil)
	c.Check(config.Hub, gc.NotNil)
	c.Check(config.WatcherFactory, gc.NotNil)
	c.Check(config.Logger, gc.Equals, s.config.Logger)
	c.Check(config.PrometheusRegisterer, gc.Equals, s.config.PrometheusRegisterer)

	c.Check(tracker.released, jc.IsFalse)
	config.Cleanup()
	c.Check(tracker.released, jc.IsTrue)
}

func (s *ManifoldSuite) TestNewWorkerErrorReleasesState(c *gc.C) {
	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state":            tracker,
		"central-hub":      pubsub.NewStructuredHub(nil),
		"multiwatcher":     &fakeMultwatcherFactory{},
		"initialized-gate": gate.NewLock(),
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, gc.ErrorMatches, "boom")
	c.Check(worker, gc.IsNil)
	c.Check(tracker.released, jc.IsTrue)
}

type fakeWorker struct {
	worker.Worker
}

type fakeMultwatcherFactory struct {
	multiwatcher.Factory
}

type fakeStateTracker struct {
	workerstate.StateTracker
	released bool
}

// Return an invalid but non-zero state pool pointer.
// Is only ever used for comparison.
func (f *fakeStateTracker) Use() (*state.StatePool, error) {
	return f.pool(), nil
}

func (f *fakeStateTracker) pool() *state.StatePool {
	return &state.StatePool{}
}

// Done tracks that the used pool is released.
func (f *fakeStateTracker) Done() error {
	f.released = true
	return nil
}
