// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"sync"

	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
)

var logger = loggo.GetLogger("juju.api.watcher")

// commonWatcher implements common watcher logic in one place to
// reduce code duplication, but it's not in fact a complete watcher;
// it's intended for embedding.
type commonWatcher struct {
	tomb tomb.Tomb
	in   chan interface{}

	// These fields must be set by the embedding watcher, before
	// calling init().

	// newResult must return a pointer to a value of the type returned
	// by the watcher's Next call.
	newResult func() interface{}

	// call should invoke the given API method, placing the call's
	// returned value in result (if any).
	call watcherAPICall
}

// watcherAPICall wraps up the information about what facade and what watcher
// Id we are calling, and just gives us a simple way to call a common method
// with a given return value.
type watcherAPICall func(method string, result interface{}) error

// makeWatcherAPICaller creates a watcherAPICall function for a given facade name
// and watcherId.
func makeWatcherAPICaller(caller base.APICaller, facadeName, watcherId string) watcherAPICall {
	bestVersion := caller.BestFacadeVersion(facadeName)
	return func(request string, result interface{}) error {
		return caller.APICall(facadeName, bestVersion,
			watcherId, request, nil, &result)
	}
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
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// When the watcher has been stopped, we send a Stop request
		// to the server, which will remove the watcher and return a
		// CodeStopped error to any currently outstanding call to
		// Next. If a call to Next happens just after the watcher has
		// been stopped, we'll get a CodeNotFound error; Either way
		// we'll return, wait for the stop request to complete, and
		// the watcher will die with all resources cleaned up.
		defer wg.Done()
		<-w.tomb.Dying()
		if err := w.call("Stop", nil); err != nil {
			logger.Errorf("error trying to stop watcher: %v", err)
		}
	}()
	wg.Add(1)
	go func() {
		// Because Next blocks until there are changes, we need to
		// call it in a separate goroutine, so the watcher can be
		// stopped normally.
		defer wg.Done()
		for {
			result := w.newResult()
			err := w.call("Next", &result)
			if err != nil {
				if params.IsCodeStopped(err) || params.IsCodeNotFound(err) {
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
	wg.Wait()
}

func (w *commonWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *commonWatcher) Err() error {
	return w.tomb.Err()
}

// notifyWatcher will send events when something changes.
// It does not send content for those changes.
type notifyWatcher struct {
	commonWatcher
	caller          base.APICaller
	notifyWatcherId string
	out             chan struct{}
}

// If an API call returns a NotifyWatchResult, you can use this to turn it into
// a local Watcher.
func NewNotifyWatcher(caller base.APICaller, result params.NotifyWatchResult) NotifyWatcher {
	w := &notifyWatcher{
		caller:          caller,
		notifyWatcherId: result.NotifyWatcherId,
		out:             make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *notifyWatcher) loop() error {
	// No results for this watcher type.
	w.newResult = func() interface{} { return nil }
	w.call = makeWatcherAPICaller(w.caller, "NotifyWatcher", w.notifyWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Since for a notifyWatcher there are no changes to send, we
		// just set the event (initial first, then after each change).
		case w.out <- struct{}{}:
		case <-w.tomb.Dying():
			return nil
		}
		if _, ok := <-w.in; !ok {
			// The tomb is already killed with the correct
			// error at this point, so just return.
			return nil
		}
	}
}

// Changes returns a channel that receives a value when a given entity
// changes in some way.
func (w *notifyWatcher) Changes() <-chan struct{} {
	return w.out
}

// stringsWatcher will send events when something changes.
// The content of the changes is a list of strings.
type stringsWatcher struct {
	commonWatcher
	caller           base.APICaller
	stringsWatcherId string
	out              chan []string
}

func NewStringsWatcher(caller base.APICaller, result params.StringsWatchResult) StringsWatcher {
	w := &stringsWatcher{
		caller:           caller,
		stringsWatcherId: result.StringsWatcherId,
		out:              make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(result.Changes))
	}()
	return w
}

func (w *stringsWatcher) loop(initialChanges []string) error {
	changes := initialChanges
	w.newResult = func() interface{} { return new(params.StringsWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "StringsWatcher", w.stringsWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Send the initial event or subsequent change.
		case w.out <- changes:
		case <-w.tomb.Dying():
			return nil
		}
		// Read the next change.
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error
			// at this point, so just return.
			return nil
		}
		changes = data.(*params.StringsWatchResult).Changes
	}
}

// Changes returns a channel that receives a list of strings of watched
// entites with changes.
func (w *stringsWatcher) Changes() <-chan []string {
	return w.out
}

// relationUnitsWatcher will sends notifications of units entering and
// leaving the scope of a RelationUnit, and changes to the settings of
// those units known to have entered.
type relationUnitsWatcher struct {
	commonWatcher
	caller                 base.APICaller
	relationUnitsWatcherId string
	out                    chan multiwatcher.RelationUnitsChange
}

func NewRelationUnitsWatcher(caller base.APICaller, result params.RelationUnitsWatchResult) RelationUnitsWatcher {
	w := &relationUnitsWatcher{
		caller:                 caller,
		relationUnitsWatcherId: result.RelationUnitsWatcherId,
		out: make(chan multiwatcher.RelationUnitsChange),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(result.Changes))
	}()
	return w
}

func (w *relationUnitsWatcher) loop(initialChanges multiwatcher.RelationUnitsChange) error {
	changes := initialChanges
	w.newResult = func() interface{} { return new(params.RelationUnitsWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "RelationUnitsWatcher", w.relationUnitsWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Send the initial event or subsequent change.
		case w.out <- changes:
		case <-w.tomb.Dying():
			return nil
		}
		// Read the next change.
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error
			// at this point, so just return.
			return nil
		}
		changes = data.(*params.RelationUnitsWatchResult).Changes
	}
}

// Changes returns a channel that will receive the changes to
// counterpart units in a relation. The first event on the channel
// holds the initial state of the relation in its Changed field.
func (w *relationUnitsWatcher) Changes() <-chan multiwatcher.RelationUnitsChange {
	return w.out
}

// machineAttachmentsWatcher will sends notifications of units entering and
// leaving the scope of a MachineStorageId, and changes to the settings of
// those units known to have entered.
type machineAttachmentsWatcher struct {
	commonWatcher
	caller                      base.APICaller
	machineAttachmentsWatcherId string
	out                         chan []params.MachineStorageId
}

// NewVolumeAttachmentsWatcher returns a MachineStorageIdsWatcher which
// communicates with the VolumeAttachmentsWatcher API facade to watch
// volume attachments.
func NewVolumeAttachmentsWatcher(caller base.APICaller, result params.MachineStorageIdsWatchResult) MachineStorageIdsWatcher {
	return newMachineStorageIdsWatcher("VolumeAttachmentsWatcher", caller, result)
}

// NewFilesystemAttachmentsWatcher returns a MachineStorageIdsWatcher which
// communicates with the FilesystemAttachmentsWatcher API facade to watch
// filesystem attachments.
func NewFilesystemAttachmentsWatcher(caller base.APICaller, result params.MachineStorageIdsWatchResult) MachineStorageIdsWatcher {
	return newMachineStorageIdsWatcher("FilesystemAttachmentsWatcher", caller, result)
}

func newMachineStorageIdsWatcher(facade string, caller base.APICaller, result params.MachineStorageIdsWatchResult) MachineStorageIdsWatcher {
	w := &machineAttachmentsWatcher{
		caller: caller,
		machineAttachmentsWatcherId: result.MachineStorageIdsWatcherId,
		out: make(chan []params.MachineStorageId),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(facade, result.Changes))
	}()
	return w
}

func (w *machineAttachmentsWatcher) loop(facade string, initialChanges []params.MachineStorageId) error {
	changes := initialChanges
	w.newResult = func() interface{} { return new(params.MachineStorageIdsWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, facade, w.machineAttachmentsWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Send the initial event or subsequent change.
		case w.out <- changes:
		case <-w.tomb.Dying():
			return nil
		}
		// Read the next change.
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error
			// at this point, so just return.
			return nil
		}
		changes = data.(*params.MachineStorageIdsWatchResult).Changes
	}
}

// Changes returns a channel that will receive the IDs of machine
// storage entity attachments which have changed.
func (w *machineAttachmentsWatcher) Changes() <-chan []params.MachineStorageId {
	return w.out
}

// EntityWatcher will send events when something changes.
// The content for the changes is a list of tag strings.
type entityWatcher struct {
	commonWatcher
	caller          base.APICaller
	entityWatcherId string
	out             chan []string
}

func NewEntityWatcher(caller base.APICaller, result params.EntityWatchResult) EntityWatcher {
	w := &entityWatcher{
		caller:          caller,
		entityWatcherId: result.EntityWatcherId,
		out:             make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(result.Changes))
	}()
	return w
}

func (w *entityWatcher) loop(initialChanges []string) error {
	changes := initialChanges
	w.newResult = func() interface{} { return new(params.EntityWatchResult) }
	w.call = makeWatcherAPICaller(w.caller, "EntityWatcher", w.entityWatcherId)
	w.commonWatcher.init()
	go w.commonLoop()

	for {
		select {
		// Send the initial event or subsequent change.
		case w.out <- changes:
		case <-w.tomb.Dying():
			return nil
		}
		// Read the next change.
		data, ok := <-w.in
		if !ok {
			// The tomb is already killed with the correct error
			// at this point, so just return.
			return nil
		}
		// Changes have been transformed at the server side already.
		changes = data.(*params.EntityWatchResult).Changes
	}
}

// Changes returns a channel that receives a list of changes
// as tags (converted to strings) of the watched entities
// with changes.
func (w *entityWatcher) Changes() <-chan []string {
	return w.out
}
