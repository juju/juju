// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/tomb"
	"sync"
)

// AllWatcher holds information allowing us to get Deltas describing changes
// to the entire environment.
type AllWatcher struct {
	client *Client
	id     *string
}

func newAllWatcher(client *Client, id *string) *AllWatcher {
	return &AllWatcher{client, id}
}

func (watcher *AllWatcher) Next() ([]params.Delta, error) {
	info := new(params.AllWatcherNextResults)
	err := watcher.client.st.Call("AllWatcher", *watcher.id, "Next", nil, info)
	return info.Deltas, err
}

func (watcher *AllWatcher) Stop() error {
	return watcher.client.st.Call("AllWatcher", *watcher.id, "Stop", nil, nil)
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
		if err := w.call("Stop", nil); err != nil {
			log.Errorf("state/api: error trying to stop watcher: %v", err)
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
				if code := ErrCode(err); code == CodeStopped || code == CodeNotFound {
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

type NotifyWatcher struct {
	commonWatcher
	st    *State
	etype string
	eid   string
	out   chan struct{}
}

func newNotifyWatcher(st *State, etype, id string) *NotifyWatcher {
	w := &NotifyWatcher{
		st:    st,
		etype: etype,
		eid:   id,
		out:   make(chan struct{}),
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
	var id params.NotifyWatcherId
	if err := w.st.Call(w.etype, w.eid, "Watch", nil, &id); err != nil {
		return err
	}
	// No results for this watcher type.
	w.newResult = func() interface{} { return nil }
	w.call = func(request string, result interface{}) error {
		return w.st.Call("NotifyWatcher", id.NotifyWatcherId, request, nil, result)
	}
	w.commonWatcher.init()
	go w.commonLoop()

	// Watch calls Next internally at the server-side, so we expect
	// changes right away.
	out := w.out
	for {
		select {
		case _, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
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

type LifecycleWatcher struct {
	commonWatcher
	st        *State
	watchCall string
	out       chan []string
}

func newLifecycleWatcher(st *State, watchCall string) *LifecycleWatcher {
	w := &LifecycleWatcher{
		st:        st,
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
	if err := w.st.Call("State", "", w.watchCall, nil, &result); err != nil {
		return err
	}
	changes := result.Ids
	w.newResult = func() interface{} { return new(params.LifecycleWatchResults) }
	w.call = func(request string, newResult interface{}) error {
		return w.st.Call("LifecycleWatcher", result.LifecycleWatcherId, request, nil, newResult)
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

type EnvironConfigWatcher struct {
	commonWatcher
	st  *State
	out chan *config.Config
}

func newEnvironConfigWatcher(st *State) *EnvironConfigWatcher {
	w := &EnvironConfigWatcher{
		st:  st,
		out: make(chan *config.Config),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *EnvironConfigWatcher) loop() error {
	var result params.EnvironConfigWatchResults
	if err := w.st.Call("State", "", "WatchEnvironConfig", nil, &result); err != nil {
		return err
	}

	envConfig, err := config.New(result.Config)
	if err != nil {
		return err
	}
	w.newResult = func() interface{} {
		return new(params.EnvironConfigWatchResults)
	}
	w.call = func(request string, newResult interface{}) error {
		return w.st.Call("EnvironConfigWatcher", result.EnvironConfigWatcherId, request, nil, newResult)
	}
	w.commonWatcher.init()
	go w.commonLoop()

	// Watch calls Next internally at the server-side, so we expect
	// changes right away.
	out := w.out
	for {
		select {
		case data, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			envConfig, err = config.New(data.(*params.EnvironConfigWatchResults).Config)
			if err != nil {
				// This should never happen, if we're talking to a compatible API server.
				log.Errorf("state/api: error reading environ config from watcher: %v", err)
				return err
			}
			out = w.out
		case out <- envConfig:
			// Wait until we have new changes to send.
			out = nil
		}
	}
	panic("unreachable")
}

// Changes returns a channel that receives the new environment
// configuration when it has changed.
func (w *EnvironConfigWatcher) Changes() <-chan *config.Config {
	return w.out
}
