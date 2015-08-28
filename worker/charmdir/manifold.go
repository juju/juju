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

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Locker controls whether the chamrdir is available or not.
type Locker interface {
	// SetAvailable sets the availability of the charm directory for clients to
	// access.
	SetAvailable(bool)

	// NotAvailable returns when all charmdir consumers are done with the
	// workload, making it "not available". If there are consumers that are
	// running functions under the premise that the charm directory is available,
	// this call will block until all of them have returned.
	NotAvailable()
}

// Consumer is used by workers that want to perform tasks on the condition that
// the charm directory is available.
type Consumer interface {
	// Run performs the given function if the charm directory is available. It
	// returns whether the charm directory was available to execute the function,
	// and any error returned by that function.
	Run(func() error) (bool, error)
}

// Manifold returns a dependency.Manifold that coordinates availability of a
// charm directory.
func Manifold() dependency.Manifold {
	return dependency.Manifold{
		Start:  startFunc,
		Output: outputFunc,
	}
}

func startFunc(_ dependency.GetResourceFunc) (worker.Worker, error) {
	w := &charmdirWorker{}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w, nil
}

func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*charmdirWorker)
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

// charmdirWorker is a degenerate worker that manages charm directory availability.
// Initial state is "not available". The "Locker" role is intended to be filled
// by the uniter, which will be responsible for setting charm directory
// availability as part of its initialization (deserialization of state on
// startup) as well as through state transitions.
type charmdirWorker struct {
	tomb tomb.Tomb

	wg sync.WaitGroup

	mu        sync.Mutex
	available bool
}

// Kill is part of the worker.Worker interface.
func (w *charmdirWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *charmdirWorker) Wait() error {
	return w.tomb.Wait()
}

// SetAvailable implements Locker.
func (u *charmdirWorker) SetAvailable(available bool) {
	u.mu.Lock()
	u.available = available
	u.mu.Unlock()
}

// Run implements Consumer.
func (u *charmdirWorker) Run(f func() error) (bool, error) {
	if u.enterAvailable() {
		defer u.exitAvailable()
		err := f()
		return true, err
	}
	return false, nil
}

func (u *charmdirWorker) enterAvailable() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.available {
		u.wg.Add(1)
		return true
	}
	return false
}

func (u *charmdirWorker) exitAvailable() {
	u.wg.Done()
}

// NotAvailable implements Locker.
func (u *charmdirWorker) NotAvailable() {
	u.mu.Lock()
	u.wg.Wait()
	u.available = false
	u.mu.Unlock()
}
