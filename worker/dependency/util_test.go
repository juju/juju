// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"time"

	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type manifoldHarness struct {
	inputs             []string
	errors             chan error
	starts             chan struct{}
	ignoreExternalKill bool
}

func newManifoldHarness(inputs ...string) *manifoldHarness {
	return &manifoldHarness{
		inputs:             inputs,
		errors:             make(chan error, 1000),
		starts:             make(chan struct{}, 1000),
		ignoreExternalKill: false,
	}
}

// newErrorIgnoringManifoldHarness starts a minimal worker that ignores
// fatal errors - and will never die.
// This is potentially nasty, but it's useful in tests where we want
// to generate fatal errors but not race on which one the engine see first.
func newErrorIgnoringManifoldHarness(inputs ...string) *manifoldHarness {
	return &manifoldHarness{
		inputs:             inputs,
		errors:             make(chan error, 1000),
		starts:             make(chan struct{}, 1000),
		ignoreExternalKill: true,
	}
}

func (ews *manifoldHarness) Manifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: ews.inputs,
		Start:  ews.start,
	}
}
func (ews *manifoldHarness) start(getResource dependency.GetResourceFunc) (worker.Worker, error) {
	for _, resourceName := range ews.inputs {
		if err := getResource(resourceName, nil); err != nil {
			return nil, err
		}
	}
	w := &minimalWorker{tomb.Tomb{}, ews.ignoreExternalKill}
	go func() {
		defer w.tomb.Done()
		ews.starts <- struct{}{}
		select {
		case <-w.tombDying():
		case err := <-ews.errors:
			w.tomb.Kill(err)
		}
	}()
	return w, nil
}

func (ews *manifoldHarness) AssertOneStart(c *gc.C) {
	ews.AssertStart(c)
	ews.AssertNoStart(c)
}

func (ews *manifoldHarness) AssertStart(c *gc.C) {
	select {
	case <-ews.starts:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never started")
	}
}

func (ews *manifoldHarness) AssertNoStart(c *gc.C) {
	select {
	case <-time.After(coretesting.ShortWait):
	case <-ews.starts:
		c.Fatalf("started unexpectedly")
	}
}

func (ews *manifoldHarness) InjectError(c *gc.C, err error) {
	select {
	case ews.errors <- err:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never sent")
	}
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
