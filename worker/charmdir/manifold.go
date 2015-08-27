// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package charmdir provides the manifold that coordinates the availability of
// a charm directory among workers. It provides two roles, a Locker who decides
// whether the charm directory is available, and a Consumer, which operates on
// an available charm directory.
package charmdir

import (
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	coreworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Locker controls whether the charmdir is available or not.
type Locker interface {
	// SetAvailable sets the availability of the charm directory for clients to
	// access.
	SetAvailable(bool)
}

// Consumer is used by workers that want to perform tasks on the condition that
// the charm directory is available.
type Consumer interface {
	// Run performs the given function if the charm directory is available. It
	// returns whether the charm directory was available to execute the function,
	// and any error returned by that function.
	Run(func() error) error
}

// Manifold returns a dependency.Manifold that coordinates availability of a
// charm directory.
func Manifold() dependency.Manifold {
	return dependency.Manifold{
		Start:  startFunc,
		Output: outputFunc,
	}
}

func startFunc(_ dependency.GetResourceFunc) (coreworker.Worker, error) {
	w := &worker{}
	go func() {
		<-w.tomb.Dying()
		// Set unavailable so that the worker waits for any outstanding callers before it's dead.
		w.SetAvailable(false)
		w.tomb.Done()
	}()
	return w, nil
}

func outputFunc(in coreworker.Worker, out interface{}) error {
	inWorker, _ := in.(*worker)
	if inWorker == nil {
		return errors.Errorf("expected %T; got %T", inWorker, in)
	}
	switch outPointer := out.(type) {
	case *Locker:
		*outPointer = inWorker
	case *Consumer:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be a pointer to a charmdir.Consumer or a charmdir.Locker; is %T", out)
	}
	return nil
}

// worker is a degenerate worker that manages charm directory availability.
// Initial state is "not available". The "Locker" role is intended to be filled
// by the uniter, which will be responsible for setting charm directory
// availability as part of its initialization (deserialization of state on
// startup) as well as through state transitions.
type worker struct {
	tomb      tomb.Tomb
	lock      sync.RWMutex
	available bool
}

// Kill is part of the worker.Worker interface.
func (w *worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *worker) Wait() error {
	return w.tomb.Wait()
}

// SetAvailable implements Locker.
func (w *worker) SetAvailable(available bool) {
	w.lock.Lock()
	w.available = available
	w.lock.Unlock()
}

// ErrNotAvailable indicates that the requested operation cannot be performed
// on the charm directory because it is not available.
var ErrNotAvailable = errors.New("charmdir not available")

// Run implements Consumer.
func (w *worker) Run(f func() error) error {
	w.lock.RLock()
	defer w.lock.RUnlock()
	if w.available {
		return f()
	}
	return ErrNotAvailable
}
