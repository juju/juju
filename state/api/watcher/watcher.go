// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/tomb"
	"sync"
)

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
		if err := w.call("Stop", nil); err != nil {
			log.Errorf("state/api: error trying to stop watcher %v", err)
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
			err := w.call("Next", &result)
			if err != nil {
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

// NotifyWatcher will send events when something changes.
// It does not send content for those changes.
type NotifyWatcher struct {
	commonWatcher
	caller          common.Caller
	notifyWatcherId string
	out             chan struct{}
}

// If an API call returns a NotifyWatchResult, you can use this to turn it into
// a local Watcher.
func NewNotifyWatcher(caller common.Caller, result params.NotifyWatchResult) *NotifyWatcher {
	w := &NotifyWatcher{
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

func (w *NotifyWatcher) loop() error {
	// No results for this watcher type.
	w.newResult = func() interface{} { return nil }
	w.call = func(request string, result interface{}) error {
		return w.caller.Call("NotifyWatcher", w.notifyWatcherId, request, nil, &result)
	}
	w.commonWatcher.init()
	go w.commonLoop()

	// The initial API call to set up the Watch should consume and
	// "transmit" the initial event. For NotifyWatchers, there is no actual
	// state transmitted, so we just set the event.
	out := w.out
	for {
		select {
		case _, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct
				// error at this point, so just return.
				return nil
			}
			// We have received changes, so send them out.
			out = w.out
		case out <- struct{}{}:
			// Wait until we have new changes to send.
			out = nil
		}
	}
	panic("unreachable")
}

// Changes returns a channel that receives a value when a given entity
// changes in some way.
func (w *NotifyWatcher) Changes() <-chan struct{} {
	return w.out
}

// StringsWatcher will send events when something changes.
// The content of the changes is a list of strings.
type StringsWatcher struct {
	commonWatcher
	caller          common.Caller
	stringWatcherId string
	out             chan []string
}

func NewStringsWatcher(caller common.Caller, result params.StringsWatchResult) *StringsWatcher {
	w := &StringsWatcher{
		caller:          caller,
		stringWatcherId: result.StringsWatcherId,
		out:             make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(result.Changes))
	}()
	return w
}

func (w *StringsWatcher) loop(initialChanges []string) error {
	changes := initialChanges
	w.newResult = func() interface{} { return new(params.StringsWatchResult) }
	w.call = func(request string, result interface{}) error {
		return w.caller.Call("StringsWatcher", w.stringWatcherId, request, nil, &result)
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
			changes = data.(*params.StringsWatchResult).Changes
			out = w.out
		case out <- changes:
			// Wait until we have new changes to send.
			out = nil
		}
	}
	panic("unreachable")
}

// Changes returns a channel that receives a list of strings of watched
// entites with changes.
func (w *StringsWatcher) Changes() <-chan []string {
	return w.out
}
