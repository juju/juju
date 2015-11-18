// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package catacomb

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
)

// Catacomb is a variant of tomb.Tomb with its own internal goroutine, designed
// for coordinating the lifetimes of private workers needed by a single parent.
//
// As a client, you should only ever create zero values; these should be used
// with Invoke to manage a parent task. No Catacomb methods are meaningful
// until the catacomb has been started with a successful Invoke.
//
// See the package documentation for more detailed discussion and usage notes.
type Catacomb struct {
	tomb  tomb.Tomb
	wg    sync.WaitGroup
	adds  chan worker.Worker
	dirty int32
}

// Plan defines the strategy for an Invoke.
type Plan struct {

	// Site must point to an unused Catacomb.
	Site *Catacomb

	// Work will be run on a new goroutine, and tracked by Site.
	Work func() error

	// Init contains additional workers for which Site must be responsible.
	Init []worker.Worker
}

// Validate returns an error if the plan cannot be used. It doesn't check for
// reused catacombs: plan validity is necessary but not sufficient to determine
// that an Invoke will succeed.
func (plan Plan) Validate() error {
	if plan.Site == nil {
		return errors.NotValidf("nil Site")
	}
	if plan.Work == nil {
		return errors.NotValidf("nil Work")
	}
	for i, w := range plan.Init {
		if w == nil {
			return errors.NotValidf("nil Init item %d", i)
		}
	}
	return nil
}

// Invoke uses the plan's catacomb to run the work func. It will return an
// error if the plan is not valid, or if the catacomb has already been used.
// If Invoke returns no error, the catacomb is now controlling the work func,
// and its exported methods can be called safely.
//
// Invoke takes responsibility for all workers in plan.Init, *whether or not
// it succeeds*.
func Invoke(plan Plan) (err error) {

	defer func() {
		if err != nil {
			stopWorkers(plan.Init)
		}
	}()

	if err := plan.Validate(); err != nil {
		return errors.Trace(err)
	}
	catacomb := plan.Site
	if !atomic.CompareAndSwapInt32(&catacomb.dirty, 0, 1) {
		return errors.Errorf("catacomb %p has already been used", catacomb)
	}
	catacomb.adds = make(chan worker.Worker)

	// Add the Init workers right away, so the client can't induce data races
	// by modifying the slice post-return.
	for _, w := range plan.Init {
		catacomb.add(w)
	}

	// This goroutine listens for added workers until the catacomb is Killed.
	// We ensure the wg can't complete until we know no new workers will be
	// added.
	catacomb.wg.Add(1)
	go func() {
		defer catacomb.wg.Done()
		for {
			select {
			case <-catacomb.tomb.Dying():
				return
			case w := <-catacomb.adds:
				catacomb.add(w)
			}
		}
	}()

	// This goroutine runs the work func and stops the catacomb with its error;
	// and waits for for the listen goroutine and all added workers to complete
	// before marking the catacomb's tomb Dead.
	go func() {
		defer catacomb.tomb.Done()
		defer catacomb.wg.Wait()
		catacomb.Kill(plan.Work())
	}()
	return nil
}

// stopWorkers stops all non-nil workers in the supplied slice, and swallows
// all errors. This is consistent, for now, because Catacomb swallows all
// errors but the first; as we come to rank or log errors, this must change
// to accommodate better practices.
func stopWorkers(workers []worker.Worker) {
	for _, w := range workers {
		if w != nil {
			worker.Stop(w)
		}
	}
}

// Add causes the supplied worker's lifetime to be bound to the catacomb's,
// relieving the client of responsibility for Kill()ing it and Wait()ing for an
// error, *whether or not this method succeeds*. If the method returns an error,
// it always indicates that the catacomb is shutting down; the value will either
// be the error from the (now-stopped) worker, or catacomb.ErrDying().
//
// If the worker completes without error, the catacomb will continue unaffected;
// otherwise the catacomb's tomb will be killed with the returned error. This
// allows clients to freely Kill() workers that have been Add()ed; any errors
// encountered will still kill the catacomb, so the workers stay under control
// until the last moment, and so can be managed pretty casually once they've
// been added.
//
// Don't try to add a worker to its own catacomb; that'll deadlock the shutdown
// procedure. I don't think there's much we can do about that.
func (catacomb *Catacomb) Add(w worker.Worker) error {
	select {
	case <-catacomb.tomb.Dying():
		if err := worker.Stop(w); err != nil {
			return errors.Trace(err)
		}
		return catacomb.ErrDying()
	case catacomb.adds <- w:
		// Note that we don't need to wait for confirmation here. This depends
		// on the catacomb.wg.Add() for the listen loop, which ensures the wg
		// won't complete until no more adds can be received.
		return nil
	}
}

// add starts two goroutines that (1) kill the catacomb's tomb with any
// error encountered by the worker; and (2) kill the worker when the
// catacomb starts dying.
func (catacomb *Catacomb) add(w worker.Worker) {

	// The coordination via stopped is not reliably observable, and hence not
	// tested, but it's yucky to leave the second goroutine running when we
	// don't need to.
	stopped := make(chan struct{})
	catacomb.wg.Add(1)
	go func() {
		defer catacomb.wg.Done()
		defer close(stopped)
		if err := w.Wait(); err != nil {
			catacomb.Kill(err)
		}
	}()
	go func() {
		select {
		case <-stopped:
		case <-catacomb.tomb.Dying():
			w.Kill()
		}
	}()
}

// Dying returns a channel that will be closed when Kill is called.
func (catacomb *Catacomb) Dying() <-chan struct{} {
	return catacomb.tomb.Dying()
}

// Dead returns a channel that will be closed when Invoke has completed (and
// thus when subsequent calls to Wait() are known not to block).
func (catacomb *Catacomb) Dead() <-chan struct{} {
	return catacomb.tomb.Dead()
}

// Wait blocks until Invoke completes, and returns the first non-nil and
// non-tomb.ErrDying error passed to Kill before Invoke finished.
func (catacomb *Catacomb) Wait() error {
	return catacomb.tomb.Wait()
}

// Kill kills the Catacomb's internal tomb with the supplied error, or one
// derived from it.
//  * if it's caused by this catacomb's ErrDying, it passes on tomb.ErrDying.
//  * if it's tomb.ErrDying, or caused by another catacomb's ErrDying, it passes
//    on a new error complaining about the misuse.
//  * all other errors are passed on unmodified.
// It's always safe to call Kill, but errors passed to Kill after the catacomb
// is dead will be ignored.
func (catacomb *Catacomb) Kill(err error) {
	if err == tomb.ErrDying {
		err = errors.New("bad catacomb Kill: tomb.ErrDying")
	}
	cause := errors.Cause(err)
	if match, ok := cause.(dyingError); ok {
		if catacomb != match.catacomb {
			err = errors.Errorf("bad catacomb Kill: other catacomb's ErrDying")
		} else {
			err = tomb.ErrDying
		}
	}

	// TODO(fwereade) it's pretty clear that this ought to be a Kill(nil), and
	// the catacomb should be responsible for ranking errors, just like the
	// dependency.Engine does, rather than determining priority by scheduling
	// alone.
	catacomb.tomb.Kill(err)
}

// ErrDying returns an error that can be used to Kill *this* catacomb without
// overwriting nil errors. It should only be used when the catacomb is already
// known to be dying; calling this method at any other time will return a
// different error, indicating client misuse.
func (catacomb *Catacomb) ErrDying() error {
	select {
	case <-catacomb.tomb.Dying():
		return dyingError{catacomb}
	default:
		return errors.New("bad catacomb ErrDying: still alive")
	}
}

// dyingError holds a reference to the catacomb that created it.
type dyingError struct {
	catacomb *Catacomb
}

// Error is part of the error interface.
func (err dyingError) Error() string {
	return fmt.Sprintf("catacomb %p is dying", err.catacomb)
}
