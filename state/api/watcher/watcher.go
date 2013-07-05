// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/tomb"
	"sync"
)

var logger = loggo.GetLogger("juju.state.api.watcher")

// Caller is an interface that just implements Call
// Most notably, Caller is implemented by *api.State
type Caller interface {
	Call(objType, id, request string, params, response interface{}) error
}

// commonWatcher implements common watcher logic in one place to
// reduce code duplication, but it's not in fact a complete watcher;
// it's intended for embedding.
type commonWatcher struct {
	tomb tomb.Tomb
	wg   sync.WaitGroup
	in   chan interface{}

	// These fields must be set by the embedding watcher, before
	// calling init().

	// newResult must return a pointer to a value of the type returned
	// by the watcher's Next call.
	newResult func() interface{}

	// call should invoke the given API method, placing the call's
	// returned value in result (if any).
	call func(method string, result interface{}) error
}

// init must be called to initialize an embedded commonWatcher's
// fields. Make sure newResult and call fields are set beforehand.
func (w *commonWatcher) init() {
	w.in = make(chan interface{})
	if w.newResult == nil {
		panic("newResult must me set")
	}
	if w.call == nil {
		panic("call must be set")
	}
}

// commonLoop implements the loop structure common to the client
// watchers. It should be started in a separate goroutine by any
// watcher that embeds commonWatcher. It kills the commonWatcher's
// tomb when an error occurs.
func (w *commonWatcher) commonLoop() {
	defer close(w.in)
	w.wg.Add(1)
	go func() {
		// When the watcher has been stopped, we send a Stop request
		// to the server, which will remove the watcher and return a
		// CodeStopped error to any currently outstanding call to
		// Next. If a call to Next happens just after the watcher has
		// been stopped, we'll get a CodeNotFound error; Either way
		// we'll return, wait for the stop request to complete, and
		// the watcher will die with all resources cleaned up.
		defer w.wg.Done()
		<-w.tomb.Dying()
		//logger.Debugf("Calling Stop for %p", w)
		if err := w.call("Stop", nil); err != nil {
			log.Errorf("state/api: error trying to stop watcher %p: %v", w, err)
		}
	}()
	w.wg.Add(1)
	go func() {
		// Because Next blocks until there are changes, we need to
		// call it in a separate goroutine, so the watcher can be
		// stopped normally.
		defer w.wg.Done()
		for {
			result := w.newResult()
			//logger.Debugf("Calling Next for %p", w)
			err := w.call("Next", &result)
			//logger.Debugf("Next returned for %p, result: %v err %v", w, result, err)
			if err != nil {
				logger.Debugf("Got error calling Next(): %v", err)
				if code := params.ErrCode(err); code == params.CodeStopped || code == params.CodeNotFound {
					if w.tomb.Err() != tomb.ErrStillAlive {
						// The watcher has been stopped at the client end, so we're
						// expecting one of the above two kinds of error.
						// We might see the same errors if the server itself
						// has been shut down, in which case we leave them
						// untouched.
						err = tomb.ErrDying
					}
				}
				// Something went wrong, just report the error and bail out.
				w.tomb.Kill(err)
				return
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.in <- result:
				// Report back the result we just got.
			}
		}
	}()
	w.wg.Wait()
}

func (w *commonWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *commonWatcher) Err() error {
	return w.tomb.Err()
}

func newEntityWatcher(caller Caller, etype, id string) params.NotifyWatcher {
	var watcherId params.NotifyWatcherId
	w := &notifyWatcher{
		caller:          caller,
		notifyWatcherId: "",
		out:             make(chan struct{}),
	}
	if err := caller.Call(etype, id, "Watch", nil, &watcherId); err != nil {
		w.tomb.Kill(err)
	}
	w.notifyWatcherId = watcherId.NotifyWatcherId
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		defer w.wg.Wait() // Wait for watcher to be stopped.
		w.tomb.Kill(w.loop())
	}()
	return w
}

type notifyWatcher struct {
	commonWatcher
	caller          Caller
	notifyWatcherId string
	out             chan struct{}
}

// If an API call returns a NotifyWatchResult, you can use this to turn it into
// a local Watcher.
func NewNotifyWatcher(caller Caller, result params.NotifyWatchResult) params.NotifyWatcher {
	w := &notifyWatcher{
		caller:          caller,
		notifyWatcherId: result.NotifyWatcherId,
		out:             make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		defer w.wg.Wait() // Wait for watcher to be stopped.
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *notifyWatcher) loop() error {
	// No results for this watcher type.
	w.newResult = func() interface{} { return nil }
	w.call = func(request string, result interface{}) error {
		return w.caller.Call("NotifyWatcher", w.notifyWatcherId, request, nil, &result)
	}
	w.commonWatcher.init()
	go w.commonLoop()

	// Watch and friends should consume their initial change, and we
	// recreate it here.
	out := w.out
	for {
		select {
		case _, ok := <-w.in:
			logger.Debugf("Got event from Next(%t) for %p", ok, w)
			if !ok {
				// The tomb is already killed with the correct
				// error at this point, so just return.
				return nil
			}
			// We have received changes, so send them out.
			out = w.out
		case out <- struct{}{}:
			// Wait until we have new changes to send.
			logger.Debugf("Sent event for %p", w)
			out = nil
		}
	}
	panic("unreachable")
}

// Changes returns a channel that receives a value when a given entity
// changes in some way.
func (w *notifyWatcher) Changes() <-chan struct{} {
	logger.Debugf("Changes requested for %p", w)
	return w.out
}

type LifecycleWatcher struct {
	commonWatcher
	caller    Caller
	watchCall string
	out       chan []string
}

func newLifecycleWatcher(caller Caller, watchCall string) *LifecycleWatcher {
	w := &LifecycleWatcher{
		caller:    caller,
		watchCall: watchCall,
		out:       make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *LifecycleWatcher) loop() error {
	var result params.LifecycleWatchResults
	if err := w.caller.Call("State", "", w.watchCall, nil, &result); err != nil {
		return err
	}
	changes := result.Ids
	w.newResult = func() interface{} { return new(params.LifecycleWatchResults) }
	w.call = func(request string, newResult interface{}) error {
		return w.caller.Call("LifecycleWatcher", result.LifecycleWatcherId, request, nil, newResult)
	}
	w.commonWatcher.init()
	go w.commonLoop()

	// The first watch call returns the initial result value which we
	// try to send immediately.
	out := w.out
	for {
		select {
		case data, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			// We have received changes, so send them out.
			changes = data.(*params.LifecycleWatchResults).Ids
			out = w.out
		case out <- changes:
			// Wait until we have new changes to send.
			out = nil
		}
	}
	panic("unreachable")
}

// Changes returns a channel that receives a list of ids of watched
// entites whose lifecycle has changed.
func (w *LifecycleWatcher) Changes() <-chan []string {
	return w.out
}
