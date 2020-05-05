// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"
)

// Manifold returns a dependency.Manifold that wraps a single channel, shared
// across all workers returned by the start func; it can be used to synchronize
// operations across manifolds that lack direct dependency relationships.
//
// The output func accepts an out pointer to either an Unlocker or a Waiter.
func Manifold() dependency.Manifold {
	return ManifoldEx(NewLock())
}

// ManifoldEx does the same thing as Manifold but takes the
// Lock which used to wait on or unlock the gate. This
// allows code running outside of a dependency engine managed worker
// to monitor or unlock the gate.
//
// TODO(mjs) - this can likely go away once all machine agent workers
// are running inside the dependency engine.
func ManifoldEx(lock Lock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ dependency.Context) (worker.Worker, error) {
			w := &gate{lock: lock}
			w.tomb.Go(func() error {
				<-w.tomb.Dying()
				return nil
			})
			return w, nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(*gate)
			if inWorker == nil {
				return errors.Errorf("in should be a *gate; is %#v", in)
			}
			switch outPointer := out.(type) {
			case *Unlocker:
				*outPointer = inWorker.lock
			case *Waiter:
				*outPointer = inWorker.lock
			case *Lock:
				*outPointer = inWorker.lock
			default:
				return errors.Errorf("out should be a *Unlocker, *Waiter, *Lock; is %#v", out)
			}
			return nil
		},
	}
}

// NewLock returns a new Lock for the gate manifold, suitable for
// passing to ManifoldEx. It can be safely unlocked and monitored by
// code running inside or outside of the dependency engine.
func NewLock() Lock {
	return &lock{
		// mu and ch are shared across all workers started by the returned manifold.
		// In normal operation, there will only be one such worker at a time; but if
		// multiple workers somehow run in parallel, mu should prevent panic and/or
		// confusion.
		mu: new(sync.Mutex),
		ch: make(chan struct{}),
	}
}

// Lock implements of Unlocker and Waiter
type lock struct {
	mu *sync.Mutex
	ch chan struct{}
}

// Unlock implements Unlocker.
func (l *lock) Unlock() {
	l.mu.Lock()
	defer l.mu.Unlock()
	select {
	case <-l.ch:
	default:
		close(l.ch)
	}
}

// Unlocked implements Waiter.
func (l *lock) Unlocked() <-chan struct{} {
	return l.ch
}

// IsUnlocked implements Waiter.
func (l *lock) IsUnlocked() bool {
	select {
	case <-l.ch:
		return true
	default:
		return false
	}
}

// gate implements a degenerate worker that holds a Lock.
type gate struct {
	tomb tomb.Tomb
	lock Lock
}

// Kill is part of the worker.Worker interface.
func (w *gate) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *gate) Wait() error {
	return w.tomb.Wait()
}
