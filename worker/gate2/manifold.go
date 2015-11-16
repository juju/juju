// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate2

import (
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Manifold returns a dependency.Manifold which implements a gate that
// can be used to synchronize the start of manifolds. The manifold's
// worker will restart when the gate is unlocked so that waiting
// dependents will be started by the dependency engine at that time.
//
// An Unlocker is also returned. This can be used anywhere to unlock
// the gate. It may be directly passed to a manifold via it's
// config. This isn't available as a convential output because the
// unlocking manifold would effectively kill itself when unlocking the
// gate!
//
// The output func accepts an out pointer to a Checker. Dependents
// should call the IsUnlocked method on the Checker to see if the gate
// is unlocked.
func Manifold() (dependency.Manifold, Unlocker) {

	ch := make(chan struct{})

	m := dependency.Manifold{
		Start: func(_ dependency.GetResourceFunc) (worker.Worker, error) {
			w := &gate{ch: ch}
			go func() {
				defer w.tomb.Done()
				w.tomb.Kill(w.wait())
			}()
			return w, nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(*gate)
			if inWorker == nil {
				return errors.Errorf("in should be a *gate; is %#v", in)
			}
			switch outPointer := out.(type) {
			case *Checker:
				*outPointer = inWorker
			default:
				return errors.Errorf("out should be a pointer to a Checker; is %#v", out)
			}
			return nil
		},
	}
	u := &unlocker{
		ch: ch,
	}
	return m, u
}

// unlocker is an implementation of Unlocker.
type unlocker struct {
	mu sync.Mutex
	ch chan struct{}
}

// Unlock implements Unlocker.
func (u *unlocker) Unlock() {
	u.mu.Lock()
	defer u.mu.Unlock()
	select {
	case <-u.ch:
	default:
		close(u.ch)
	}
}

// Unlocked implements Unlocker.
func (u *unlocker) Unlocked() <-chan struct{} {
	return u.ch
}

// gate implements Checker and worker.Worker.
type gate struct {
	tomb tomb.Tomb
	ch   chan struct{}
}

func (w *gate) wait() error {
	// If the gate is unblocked (channel has been closed), don't
	// select on it.
	ch := w.ch
	if w.IsUnlocked() {
		ch = nil
	}

	select {
	case err := <-w.tomb.Dying():
		return tomb.ErrDying
	case <-ch:
		return errors.New("gate closed")
	}
}

// Kill is part of the worker.Worker interface.
func (w *gate) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *gate) Wait() error {
	return w.tomb.Wait()
}

// IsUnlocked is part of the Checker interface.
func (w *gate) IsUnlocked() bool {
	select {
	case <-w.ch:
		return true
	default:
		return false
	}
}
