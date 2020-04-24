// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	"unsafe"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/multiwatcher"
	workerstate "github.com/juju/juju/worker/state"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config multiwatcher.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = multiwatcher.ManifoldConfig{
		StateName:            "state",
		Logger:               loggo.GetLogger("test"),
		PrometheusRegisterer: noopRegisterer{},
		NewWorker: func(multiwatcher.Config) (worker.Worker, error) {
			return nil, errors.New("boom")
		},
		NewAllWatcher: func(*state.StatePool) state.AllWatcherBacking {
			return &fakeAllWatcher{}
		},
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return multiwatcher.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold().Inputs, jc.DeepEquals, []string{"state"})
}

func (s *ManifoldSuite) TestConfigValidation(c *gc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingStateName(c *gc.C) {
	s.config.StateName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "empty StateName not valid")
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
		"state": dependency.ErrMissing,
	})

	w, err := s.manifold().Start(context)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *gc.C) {
	var config multiwatcher.Config
	s.config.NewWorker = func(c multiwatcher.Config) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state": tracker,
	})

	w, err := s.manifold().Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.NotNil)

	c.Check(config.Validate(), jc.ErrorIsNil)
	c.Check(config.Logger, gc.Equals, s.config.Logger)
	c.Check(config.PrometheusRegisterer, gc.Equals, s.config.PrometheusRegisterer)

	c.Check(tracker.released, jc.IsFalse)
	config.Cleanup()
	c.Check(tracker.released, jc.IsTrue)
}

func (s *ManifoldSuite) TestNewWorkerErrorReleasesState(c *gc.C) {
	tracker := &fakeStateTracker{}
	context := dt.StubContext(nil, map[string]interface{}{
		"state": tracker,
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, gc.ErrorMatches, "boom")
	c.Check(worker, gc.IsNil)
	c.Check(tracker.released, jc.IsTrue)
}

type fakeWorker struct {
	worker.Worker
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

// pool returns a non-nil but invalid pointer to a state pool.
func (f *fakeStateTracker) pool() *state.StatePool {
	return (*state.StatePool)(unsafe.Pointer(f))
}

// Done tracks that the used pool is released.
func (f *fakeStateTracker) Done() error {
	f.released = true
	return nil
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}

type fakeAllWatcher struct {
	state.AllWatcherBacking
}
