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

type errorWorkerStarter struct {
	inputs []string
	errors chan error
	starts chan struct{}
}

func newErrorWorkerStarter(inputs ...string) *errorWorkerStarter {
	return &errorWorkerStarter{
		inputs: inputs,
		errors: make(chan error, 1000),
		starts: make(chan struct{}, 1000),
	}
}

func (ews *errorWorkerStarter) Manifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: ews.inputs,
		Start:  ews.start,
	}
}

func (ews *errorWorkerStarter) start(getResource dependency.GetResourceFunc) (worker.Worker, error) {
	for _, resourceName := range ews.inputs {
		if !getResource(resourceName, nil) {
			return nil, dependency.ErrUnmetDependencies
		}
	}
	w := &degenerateWorker{}
	go func() {
		defer w.tomb.Done()
		ews.starts <- struct{}{}
		select {
		case <-w.tomb.Dying():
		case err := <-ews.errors:
			w.tomb.Kill(err)
		}
	}()
	return w, nil
}

func (ews *errorWorkerStarter) AssertOneStart(c *gc.C) {
	ews.AssertStart(c)
	ews.AssertNoStart(c)
}

func (ews *errorWorkerStarter) AssertStart(c *gc.C) {
	select {
	case <-ews.starts:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never started")
	}
}

func (ews *errorWorkerStarter) AssertNoStart(c *gc.C) {
	select {
	case <-time.After(coretesting.ShortWait):
	case <-ews.starts:
		c.Fatalf("started unexpectedly")
	}
}

func (ews *errorWorkerStarter) InjectError(c *gc.C, err error) {
	select {
	case ews.errors <- err:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never sent")
	}
}

type degenerateWorker struct {
	tomb tomb.Tomb
}

func (w *degenerateWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *degenerateWorker) Wait() error {
	return w.tomb.Wait()
}

func degenerateStart(_ dependency.GetResourceFunc) (worker.Worker, error) {
	w := &degenerateWorker{}
	go func() {
		<-w.tomb.Dying()
		w.tomb.Done()
	}()
	return w, nil
}

func nothingFatal(_ error) bool {
	return false
}
