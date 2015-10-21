// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type engineFixture struct {
	testing.IsolationSuite
	engine dependency.Engine
}

func (s *engineFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.startEngine(c, nothingFatal)
}

func (s *engineFixture) TearDownTest(c *gc.C) {
	s.stopEngine(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *engineFixture) startEngine(c *gc.C, isFatal dependency.IsFatalFunc) {
	if s.engine != nil {
		c.Fatalf("original engine not stopped")
	}
	config := dependency.EngineConfig{
		IsFatal:     isFatal,
		WorstError:  func(err0, err1 error) error { return err0 },
		ErrorDelay:  coretesting.ShortWait / 2,
		BounceDelay: coretesting.ShortWait / 10,
	}

	e, err := dependency.NewEngine(config)
	c.Assert(err, jc.ErrorIsNil)
	s.engine = e
}

func (s *engineFixture) stopEngine(c *gc.C) {
	if s.engine != nil {
		err := worker.Stop(s.engine)
		s.engine = nil
		c.Check(err, jc.ErrorIsNil)
	}
}

type manifoldHarness struct {
	inputs             []string
	errors             chan error
	starts             chan struct{}
	requireResources   bool
	ignoreExternalKill bool
}

func newManifoldHarness(inputs ...string) *manifoldHarness {
	return &manifoldHarness{
		inputs:           inputs,
		errors:           make(chan error, 1000),
		starts:           make(chan struct{}, 1000),
		requireResources: true,
	}
}

func newResourceIgnoringManifoldHarness(inputs ...string) *manifoldHarness {
	mh := newManifoldHarness(inputs...)
	mh.requireResources = false
	return mh
}

// newErrorIgnoringManifoldHarness starts a minimal worker that ignores
// fatal errors - and will never die.
// This is potentially nasty, but it's useful in tests where we want
// to generate fatal errors but not race on which one the engine see first.
func newErrorIgnoringManifoldHarness(inputs ...string) *manifoldHarness {
	mh := newManifoldHarness(inputs...)
	mh.ignoreExternalKill = true
	return mh
}

func (mh *manifoldHarness) Manifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: mh.inputs,
		Start:  mh.start,
	}
}

func (mh *manifoldHarness) start(getResource dependency.GetResourceFunc) (worker.Worker, error) {
	for _, resourceName := range mh.inputs {
		if err := getResource(resourceName, nil); err != nil {
			if mh.requireResources {
				return nil, err
			}
		}
	}
	w := &minimalWorker{tomb.Tomb{}, mh.ignoreExternalKill}
	go func() {
		defer w.tomb.Done()
		mh.starts <- struct{}{}
		select {
		case <-w.tombDying():
		case err := <-mh.errors:
			w.tomb.Kill(err)
		}
	}()
	return w, nil
}

func (mh *manifoldHarness) AssertOneStart(c *gc.C) {
	mh.AssertStart(c)
	mh.AssertNoStart(c)
}

func (mh *manifoldHarness) AssertStart(c *gc.C) {
	select {
	case <-mh.starts:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never started")
	}
}

func (mh *manifoldHarness) AssertNoStart(c *gc.C) {
	select {
	case <-time.After(coretesting.ShortWait):
	case <-mh.starts:
		c.Fatalf("started unexpectedly")
	}
}

func (mh *manifoldHarness) InjectError(c *gc.C, err error) {
	select {
	case mh.errors <- err:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never sent")
	}
}

func newTracedManifoldHarness(inputs ...string) *tracedManifoldHarness {
	return &tracedManifoldHarness{
		&manifoldHarness{
			inputs:             inputs,
			errors:             make(chan error, 1000),
			starts:             make(chan struct{}, 1000),
			ignoreExternalKill: false,
		},
	}
}

type tracedManifoldHarness struct {
	*manifoldHarness
}

func (mh *tracedManifoldHarness) Manifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: mh.inputs,
		Start:  mh.start,
	}
}

func (mh *tracedManifoldHarness) start(getResource dependency.GetResourceFunc) (worker.Worker, error) {
	for _, resourceName := range mh.inputs {
		if err := getResource(resourceName, nil); err != nil {
			return nil, errors.Trace(err)
		}
	}
	w := &minimalWorker{tomb.Tomb{}, mh.ignoreExternalKill}
	go func() {
		defer w.tomb.Done()
		mh.starts <- struct{}{}
		select {
		case <-w.tombDying():
		case err := <-mh.errors:
			w.tomb.Kill(err)
		}
	}()
	return w, nil
}

type minimalWorker struct {
	tomb               tomb.Tomb
	ignoreExternalKill bool
}

func (w *minimalWorker) tombDying() <-chan struct{} {
	if w.ignoreExternalKill {
		return nil
	}
	return w.tomb.Dying()
}

func (w *minimalWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *minimalWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *minimalWorker) Report() map[string]interface{} {
	return map[string]interface{}{
		"key1": "hello there",
	}
}

func startMinimalWorker(_ dependency.GetResourceFunc) (worker.Worker, error) {
	w := &minimalWorker{}
	go func() {
		<-w.tomb.Dying()
		w.tomb.Done()
	}()
	return w, nil
}

func nothingFatal(_ error) bool {
	return false
}
