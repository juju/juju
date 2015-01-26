// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hook

// Source defines a generator of Info events.
// A Source's client must be careful to:
//  * use it from a single goroutine.
//  * collect asynchronous events from Changes(), and synchronously Apply()
//    them whenever possible.
//  * only use fresh values returned from Empty() and Next(); i.e. only those
//    which have been generated since the last call to Pop() or Update().
//  * Stop() it when finished.
type Source interface {

	// Changes returns a channel sending events which must be processed
	// synchronously, and in order.
	Changes() <-chan SourceChange

	// Stop causes the HookSource to clean up its resources and stop sending
	// changes.
	Stop() error

	// Empty returns false if any hooks are scheduled.
	Empty() bool

	// Next returns the first scheduled hook. It will panic if no hooks are
	// scheduled.
	Next() Info

	// Pop removes the first scheduled hook, and invalidates the results of
	// previous calls to Next() and Empty(). It will panic if no hooks are
	// scheduled.
	Pop()
}

// SourceChange is a function that is returned via Source.Changes().
type SourceChange func() error

// Apply applies the change to its Source's schedule, and invalidates the
// results of previous calls to Next() and Empty().
func (s SourceChange) Apply() error {
	return s()
}

// NoUpdates implements a subset of Source that delivers no changes, errors
// on update, and ignores stop requests (because there's nothing running in the
// first place). It's useful for implementing static Sources.
type NoUpdates struct{}

func (_ *NoUpdates) Stop() error                  { return nil }
func (_ *NoUpdates) Changes() <-chan SourceChange { return nil }
