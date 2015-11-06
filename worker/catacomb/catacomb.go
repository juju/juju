// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package catacomb

import (
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
)

// ErrDying is used to signal acquiescence to a Catacomb's Dying notification.
// As with tomb.ErrDying, it must not be used in *any* other circumstances, lest
// it somehow leak into a worker that isn't really dying and cause a panic.
var ErrDying = errors.New("catacomb is dying")

// Catacomb is a variant of tomb.Tomb with its own internal goroutine, designed
// for coordinating the lifetimes of a set of private workers needed by a single
// parent. See the package documentation for detailed discussion and usage notes.
type Catacomb struct {
	tomb     tomb.Tomb
	wg       sync.WaitGroup
	requests chan addRequest
}

// addRequest holds a worker that should be added to a catacomb, and a channel
// that will be closed to confirm successful addition.
type addRequest struct {
	worker worker.Worker
	reply  chan<- struct{}
}

// New creates a new Catacomb. The caller is reponsible for calling Done exactly
// once (twice will panic; never will leak a goroutine).
func New() *Catacomb {
	catacomb := &Catacomb{
		requests: make(chan addRequest),
	}
	go catacomb.loop()
	return catacomb
}

// loop registers added workers and waits for death.
func (catacomb *Catacomb) loop() {
	for {
		select {
		case <-catacomb.tomb.Dying():
			return
		case request := <-catacomb.requests:
			catacomb.add(request)
		}
	}
}

// Add causes the supplied worker's lifetime to be bound to that of the Catacomb,
// relieving the client of responsibility for Kill()ing it and Wait()ing for an
// error, *whether or not this method succeeds*.
//
// If the worker completes without error, the catacomb will continue unaffected;
// otherwise the catacomb's tomb will be killed with the returned error. This
// allows clients to freely Kill() workers that have been Add()ed; any errors
// encountered will still kill the catacomb, so the workers stay under control
// until the last moment, and so can be managed pretty casually once they've
// been added.
func (catacomb *Catacomb) Add(w worker.Worker) error {
	reply := make(chan struct{})
	requests := catacomb.requests
	for {
		select {
		case <-catacomb.tomb.Dying():
			if err := worker.Stop(w); err != nil {
				return errors.Trace(err)
			}
			return errors.New("catacomb not alive")
		case requests <- addRequest{w, reply}:
			requests = nil
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

// Kill kills the Catacomb's internal tomb with the supplied error; *unless*
// the error's Cause is ErrDying or tomb.ErrDying, in which case it passes on
// tomb.ErrDying instead (which will panic if the tomb isn't already dying).
func (catacomb *Catacomb) Kill(err error) {
	switch cause := errors.Cause(err); cause {
	case ErrDying, tomb.ErrDying:
		err = tomb.ErrDying
	}
	catacomb.tomb.Kill(err)
}

// Dying returns a channel that will be closed when Kill is called.
func (catacomb *Catacomb) Dying() <-chan struct{} {
	return catacomb.tomb.Dying()
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

// Dead returns a channel that will be closed when Done is called (and thus
// when subsequent calls to Wait() are known not to block).
func (catacomb *Catacomb) Dead() <-chan struct{} {
	return catacomb.tomb.Dead()
}

// Wait blocks until someone calls Done, and returns the first non-nil and
// non-tomb.ErrDying error passed to Kill before Done was called.
func (catacomb *Catacomb) Wait() error {
	return catacomb.tomb.Wait()
}
