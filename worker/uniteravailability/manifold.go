// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniteravailability provides the manifold that maintains information
// about whether the uniter is available or not.
package uniteravailability

import (
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// UniterAvailabilitySetter interface defines the function for setting
// the state of the uniter (a boolean signifying whether it is available or not).
type UniterAvailabilitySetter interface {
	// SetAvailable marks the workload as not available.
	SetAvailable(bool)

	// NotAvailable returns when all availability consumers are done with the
	// workload. If there are consumers This call will block until this condition is met.
	NotAvailable()
}

// UniterAvailabilityConsumer interface defines the functions available
// to consumers that depend on the workload being available for asynchronous activity.
type UniterAvailabilityConsumer interface {
	// EnterAvailable returns whether the uniter is available, and enters an
	// availability critical section that will be mutually exclusive with
	// upgrades and other uniter operations that must be restricted.
	//
	// If the uniter is available, this caller is responsible for calling
	// ExitAvailable when done with the operation that depends on availability.
	EnterAvailable() bool

	// ExitAvailable releases the availability back to the uniter when the
	// consumer is finished. This call should ONLY be called if the consumer was
	// able to enter availability in the first place.
	ExitAvailable()
}

// Manifold returns a dependency.Manifold that keeps track of whether the uniter
// is available.
func Manifold() dependency.Manifold {
	return dependency.Manifold{
		Start:  startFunc,
		Output: outputFunc,
	}
}

func startFunc(_ dependency.GetResourceFunc) (worker.Worker, error) {
	w := &availabilityWorker{}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w, nil
}

func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*availabilityWorker)
	if inWorker == nil {
		return errors.Errorf("expected %T; got %T", inWorker, in)
	}
	switch outPointer := out.(type) {
	case *UniterAvailabilitySetter:
		*outPointer = inWorker
	case *UniterAvailabilityConsumer:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be a pointer to a UniterAvailabilityConsumer or a UniterAvailabilitySetter; is %T", out)
	}
	return nil
}

// availabilityWorker is a degenerate worker that manages uniter availability.
// Initial state is "not available". The uniter is responsible for setting
// availability as part of its initialization (deserialization of state on
// startup) as well as through state transitions.
type availabilityWorker struct {
	tomb tomb.Tomb

	wg sync.WaitGroup

	mu        sync.Mutex
	available bool
}

// Kill is part of the worker.Worker interface.
func (w *availabilityWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *availabilityWorker) Wait() error {
	return w.tomb.Wait()
}

// Available implements UniterAvailabilityConsumer.
func (u *availabilityWorker) Available() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.available
}

// Available implements UniterAvailabilitySetter.
func (u *availabilityWorker) SetAvailable(available bool) {
	u.mu.Lock()
	u.available = available
	u.mu.Unlock()
}

// EnterAvailable implements UniterAvailabilityConsumer.
func (u *availabilityWorker) EnterAvailable() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.available {
		u.wg.Add(1)
		return true
	}
	return false
}

// ExitAvailable implements UniterAvailabilityConsumer.
func (u *availabilityWorker) ExitAvailable() {
	u.wg.Done()
}

// NotAvailable implements UniterAvailailitySetter.
func (u *availabilityWorker) NotAvailable() {
	u.mu.Lock()
	u.wg.Wait()
	u.available = false
	u.mu.Unlock()
}
