// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

// Multiwatcher watches any changes to the state.
type Multiwatcher struct {
	all *storeManager

	// used indicates that the watcher was used (i.e. Next() called).
	used bool

	// The following fields are maintained by the storeManager
	// goroutine.
	revno   int64
	stopped bool
}

// NewMultiwatcher creates a new watcher that can observe
// changes to an underlying store manager.
func NewMultiwatcher(all *storeManager) *Multiwatcher {
	// Note that we want to be clear about the defaults. So we set zero
	// values explicitly.
	//  used:    false means that the watcher has not been used yet
	//  revno:   0 means that *all* transactions prior to the first
	//           Next() call will be reflected in the deltas.
	//  stopped: false means that the watcher immediately starts off
	//           handling changes.
	return &Multiwatcher{
		all:     all,
		used:    false,
		revno:   0,
		stopped: false,
	}
}

// Stop stops the watcher.
func (w *Multiwatcher) Stop() error {
	select {
	case w.all.request <- &request{w: w}:
		return nil
	case <-w.all.tomb.Dead():
	}
	return errors.Trace(w.all.tomb.Err())
}

// Next retrieves all changes that have happened since the last
// time it was called, blocking until there are some changes available.
//
// The result from the initial call to Next() is different from
// subsequent calls. The latter will reflect changes that have happened
// since the last Next() call. In contrast, the initial Next() call will
// return the deltas that represent the model's complete state at that
// moment, even when the model is empty. In that empty model case an
// empty set of deltas is returned.
func (w *Multiwatcher) Next() ([]multiwatcher.Delta, error) {
	req := &request{
		w:     w,
		reply: make(chan bool),
	}
	if !w.used {
		req.noChanges = make(chan struct{})
		w.used = true
	}

	select {
	case <-w.all.tomb.Dying():
		err := w.all.tomb.Err()
		if err == nil {
			err = multiwatcher.ErrStoppedf("shared state watcher")
		}
		return nil, err
	case w.all.request <- req:
	}

	// TODO(ericsnow) Clean up Multiwatcher/storeManager interaction.
	// Relying on req.reply and req.noChanges here is not an ideal
	// solution. It reflects the level of coupling we have between
	// the Multiwatcher, request, and storeManager types.
	select {
	case <-w.all.tomb.Dying():
		err := w.all.tomb.Err()
		if err == nil {
			err = multiwatcher.ErrStoppedf("shared state watcher")
		}
		return nil, err
	case ok := <-req.reply:
		if !ok {
			return nil, errors.Trace(multiwatcher.NewErrStopped())
		}
	case <-req.noChanges:
		return []multiwatcher.Delta{}, nil
	}
	return req.changes, nil
}

// storeManager holds a shared record of current state and replies to
// requests from Multiwatchers to tell them when it changes.
type storeManager struct {
	tomb tomb.Tomb

	// backing knows how to fetch information from
	// the underlying state.
	backing Backing

	// request receives requests from Multiwatcher clients.
	request chan *request

	// store holds information on everything the storeManager cares about.
	store multiwatcher.Store

	// Each entry in the waiting map holds a linked list of Next requests
	// outstanding for the associated watcher.
	waiting map[*Multiwatcher]*request
}

// Backing is the interface required by the storeManager to access the
// underlying state.
type Backing interface {
	// GetAll retrieves information about all information
	// known to the Backing and stashes it in the Store.
	GetAll(multiwatcher.Store) error

	// Changed informs the backing about a change received
	// from a watcher channel.  The backing is responsible for
	// updating the Store to reflect the change.
	Changed(multiwatcher.Store, watcher.Change) error

	// Watch watches for any changes and sends them
	// on the given channel.
	Watch(chan<- watcher.Change)

	// Unwatch stops watching for changes on the
	// given channel.
	Unwatch(chan<- watcher.Change)

	// Release cleans up resources opened by the Backing.
	Release() error
}

// request holds a message from the Multiwatcher to the
// storeManager for some changes. The request will be
// replied to when some changes are available.
type request struct {
	// w holds the Multiwatcher that has originated the request.
	w *Multiwatcher

	// reply receives a message when deltas are ready.  If reply is
	// nil, the Multiwatcher will be stopped.  If the reply is true,
	// the request has been processed; if false, the Multiwatcher
	// has been stopped,
	reply chan bool

	// noChanges receives a message when the manager checks for changes
	// and there are none.
	noChanges chan struct{}

	// On reply, changes will hold changes that have occurred since
	// the last replied-to Next request.
	changes []multiwatcher.Delta

	// next points to the next request in the list of outstanding
	// requests on a given watcher.  It is used only by the central
	// storeManager goroutine.
	next *request
}

// newStoreManagerNoRun creates the store manager
// but does not start its run loop.
func newStoreManagerNoRun(backing Backing) *storeManager {
	return &storeManager{
		backing: backing,
		request: make(chan *request),
		store:   multiwatcher.NewStore(loggo.GetLogger("juju.multiwatcher.store")),
		waiting: make(map[*Multiwatcher]*request),
	}
}

// newDeadStoreManager returns a store manager instance
// that is already dead and always returns the given error.
func newDeadStoreManager(err error) *storeManager {
	var m storeManager
	m.tomb.Kill(errors.Trace(err))
	return &m
}

// newStoreManager returns a new storeManager that retrieves information
// using the given backing.
func newStoreManager(backing Backing) *storeManager {
	sm := newStoreManagerNoRun(backing)
	sm.tomb.Go(func() error {
		// TODO(rog) distinguish between temporary and permanent errors:
		// if we get an error in loop, this logic kill the state's storeManager
		// forever. This currently fits the way we go about things,
		// because we reconnect to the state on any error, but
		// perhaps there are errors we could recover from.

		err := sm.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			logger.Infof("store manager loop failed: %v", err)
		}
		return cause
	})
	return sm
}

func (sm *storeManager) loop() error {
	in := make(chan watcher.Change)
	sm.backing.Watch(in)
	defer sm.backing.Unwatch(in)
	// We have no idea what changes the watcher might be trying to
	// send us while getAll proceeds, but we don't mind, because
	// storeManager.changed is idempotent with respect to both updates
	// and removals.
	// TODO(rog) Perhaps find a way to avoid blocking all other
	// watchers while GetAll is running.
	if err := sm.backing.GetAll(sm.store); err != nil {
		return err
	}
	for {
		select {
		case <-sm.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case change := <-in:
			if err := sm.backing.Changed(sm.store, change); err != nil {
				return errors.Trace(err)
			}
		case req := <-sm.request:
			sm.handle(req)
		}
		sm.respond()
	}
}

// Kill implements worker.Worker.Kill.
func (sm *storeManager) Kill() {
	sm.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (sm *storeManager) Wait() error {
	return errors.Trace(sm.tomb.Wait())
}

// Stop stops the storeManager.
func (sm *storeManager) Stop() error {
	return worker.Stop(sm)
}

// handle processes a request from a Multiwatcher to the storeManager.
func (sm *storeManager) handle(req *request) {
	if req.w.stopped {
		// The watcher has previously been stopped.
		if req.reply != nil {
			select {
			case req.reply <- false:
			case <-sm.tomb.Dying():
			}
		}
		return
	}
	if req.reply == nil {
		// This is a request to stop the watcher.
		for req := sm.waiting[req.w]; req != nil; req = req.next {
			select {
			case req.reply <- false:
			case <-sm.tomb.Dying():
			}
		}
		delete(sm.waiting, req.w)
		req.w.stopped = true
		sm.store.DecReference(req.w.revno)
		return
	}
	// Add request to head of list.
	req.next = sm.waiting[req.w]
	sm.waiting[req.w] = req
}

// respond responds to all outstanding requests that are satisfiable.
func (sm *storeManager) respond() {
	for w, req := range sm.waiting {
		revno := w.revno
		changes, latestRevno := sm.store.ChangesSince(revno)
		if len(changes) == 0 {
			if req.noChanges != nil {
				select {
				case req.noChanges <- struct{}{}:
				case <-sm.tomb.Dying():
					return
				}

				sm.removeWaitingReq(w, req)
			}
			continue
		}

		req.changes = changes
		w.revno = latestRevno

		select {
		case req.reply <- true:
		case <-sm.tomb.Dying():
			return
		}

		sm.removeWaitingReq(w, req)
		sm.store.AddReference(revno)
	}
}

func (sm *storeManager) removeWaitingReq(w *Multiwatcher, req *request) {
	if req := req.next; req == nil {
		// Last request for this watcher.
		delete(sm.waiting, w)
	} else {
		sm.waiting[w] = req
	}
}
