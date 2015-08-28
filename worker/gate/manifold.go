// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate

import (
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Manifold returns a dependency.Manifold that wraps a single channel, shared
// across all workers returned by the start func; it can be used to synchronize
// operations acrosss manifolds that lack direct dependency relationships.
//
// The output func accepts an out pointer to either an Unlocker or a Waiter.
func Manifold() dependency.Manifold {

	// mu and ch are shared across all workers started by the returned manifold.
	// In normal operation, there will only be one such worker at a time; but if
	// multiple workers somehow run in parallel, mu should prevent panic and/or
	// confusion.
	mu := new(sync.Mutex)
	ch := make(chan struct{})

	return dependency.Manifold{
		Start: func(_ dependency.GetResourceFunc) (worker.Worker, error) {
			w := &gate{
				mu: mu,
				ch: ch,
			}
			go func() {
				defer w.tomb.Done()
				<-w.tomb.Dying()
			}()
			return w, nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(*gate)
			if inWorker == nil {
				return errors.Errorf("in should be a *gate; is %#v", in)
			}
			switch outPointer := out.(type) {
			case *Unlocker:
				*outPointer = inWorker
			case *Waiter:
				*outPointer = inWorker
			default:
				return errors.Errorf("out should be a pointer to an Unlocker or a Waiter; is %#v", out)
			}
			return nil
		},
	}
}

// gate implements Waiter, Unlocker, and worker.Worker.
type gate struct {
	tomb tomb.Tomb
	mu   *sync.Mutex
	ch   chan struct{}
}

// Kill is part of the worker.Worker interface.
func (w *gate) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *gate) Wait() error {
	return w.tomb.Wait()
}

// Unlocked is part of the Waiter interface.
func (w *gate) Unlocked() <-chan struct{} {
	return w.ch
}

// Unlock is part of the Unlocker interface.
func (w *gate) Unlock() {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.ch:
	default:
		close(w.ch)
	}
}
