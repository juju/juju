// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package catacomb

import (
	"fmt"
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
)

// Catacomb is a variant of tomb.Tomb with its own internal goroutine, designed
// for coordinating the lifetimes of a set of private workers needed by a single
// parent. See the package documentation for detailed discussion and usage notes.
type Catacomb struct {
	tomb tomb.Tomb
	wg   sync.WaitGroup
	adds chan addRequest
}

// addRequest holds a worker to be added and a channel to close when the
// addition is confirmed.
type addRequest struct {
	worker worker.Worker
	reply  chan struct{}
}

// New creates a new Catacomb. The caller is reponsible for calling Done exactly
// once (twice will panic; never will leak a goroutine).
func New() *Catacomb {
	catacomb := &Catacomb{
		adds: make(chan addRequest),
	}
	catacomb.wg.Add(1)
	go func() {
		defer catacomb.wg.Done()
		catacomb.loop()
	}()
	return catacomb
}

// loop registers added workers and waits for death. It must be called exactly
// once, on its own goroutine.
func (catacomb *Catacomb) loop() {
	for {
		select {
		case <-catacomb.tomb.Dying():
			return
		case request := <-catacomb.adds:
			catacomb.add(request)
		}
	}
}

// Add causes the supplied worker's lifetime to be bound to that of the Catacomb,
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
func (catacomb *Catacomb) Add(w worker.Worker) error {
	adds := catacomb.adds
	reply := make(chan struct{})
	for {
		select {
		case <-catacomb.tomb.Dying():
			if err := worker.Stop(w); err != nil {
				return errors.Trace(err)
			}
			return catacomb.ErrDying()
		case adds <- addRequest{w, reply}:
			adds = nil
		case <-reply:
			return nil
		}
	}
}

// add starts two goroutines that (1) kill the catacomb's tomb with any
// error encountered by the worker; and (2) kill the worker when the
// catacomb starts dying. The reply channel is closed only once the worker
// has been recorded in the catacomb's WaitGroup.
func (catacomb *Catacomb) add(request addRequest) {
	catacomb.wg.Add(1)
	close(request.reply)

	// The coordination via stopped is not externally observable, and hence
	// not tested, but it's yucky to leave the second goroutine running when
	// we don't need to.
	stopped := make(chan struct{})
	go func() {
		defer catacomb.wg.Done()
		defer close(stopped)
		if err := request.worker.Wait(); err != nil {
			catacomb.tomb.Kill(err)
		}
	}()
	go func() {
		select {
		case <-stopped:
		case <-catacomb.tomb.Dying():
			request.worker.Kill()
		}
	}()
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
	catacomb.tomb.Kill(err)
}

// Dying returns a channel that will be closed when Kill is called.
func (catacomb *Catacomb) Dying() <-chan struct{} {
	return catacomb.tomb.Dying()
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

// Done kills the Catacomb's internal tomb (with no error), waits for all added
// workers to complete, and records their errors, before marking the tomb Dead
// and impervious to further changes.
// It is safe to call Done without having called Kill.
// It is incorrect to call Done more than once.
func (catacomb *Catacomb) Done() {
	catacomb.tomb.Kill(nil)
	catacomb.wg.Wait()
	catacomb.tomb.Done()
}

// Dead returns a channel that will be closed when Done has completed (and thus
// when subsequent calls to Wait() are known not to block).
func (catacomb *Catacomb) Dead() <-chan struct{} {
	return catacomb.tomb.Dead()
}

// Wait blocks until someone calls Done, and returns the first non-nil and
// non-tomb.ErrDying error passed to Kill before Done was called.
func (catacomb *Catacomb) Wait() error {
	return catacomb.tomb.Wait()
}

// dyingError holds a reference to the catacomb that created it.
type dyingError struct {
	catacomb *Catacomb
}

// Error is part of the error interface.
func (err dyingError) Error() string {
	return fmt.Sprintf("catacomb %p is dying", err.catacomb)
}
