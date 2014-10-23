// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
)

// HookSource defines a generator of hook.Info events. A HookSource's client
// must be careful to:
//  * use it from a single goroutine.
//  * collect asynchronous events from Changes(), and synchronously pass
//    them to Update() whenever possible.
//  * only use fresh values returned from Empty() and Next(); i.e. only those
//    which have been generated since the last call to Pop() or Update().
//  * Stop() it when finished.
type HookSource interface {

	// Changes returns a channel sending events which must be delivered --
	// synchronously, and in order -- to the HookSource's Update method .
	Changes() <-chan params.RelationUnitsChange

	// Stop causes the HookSource to clean up its resources and stop sending
	// changes.
	Stop() error

	// Update applies the supplied change to the HookSource's schedule, and
	// invalidates the results of previous calls to Next() and Empty(). It
	// should only be called with change events received from the Changes()
	// channel.
	Update(change params.RelationUnitsChange) error

	// Empty returns false if any hooks are scheduled.
	Empty() bool

	// Next returns the first scheduled hook. It will panic if no hooks are
	// scheduled.
	Next() hook.Info

	// Pop removes the first scheduled hook, and invalidates the results of
	// previous calls to Next() and Empty(). It will panic if no hooks are
	// scheduled.
	Pop()
}

// NoUpdates implements a subset of HookSource that delivers no changes, errors
// on update, and ignores stop requests (because there's nothing running in the
// first place). It's useful for implementing static HookSources.
type NoUpdates struct{}

func (_ *NoUpdates) Stop() error                                { return nil }
func (_ *NoUpdates) Changes() <-chan params.RelationUnitsChange { return nil }
func (_ *NoUpdates) Update(_ params.RelationUnitsChange) error {
	return errors.Errorf("HookSource does not accept updates")
}

// RelationUnitsWatcher produces RelationUnitsChange events until stopped, or
// until it encounters an error. It must not close its Changes channel without
// signalling an error via Stop and Err.
type RelationUnitsWatcher interface {
	Err() error
	Stop() error
	Changes() <-chan params.RelationUnitsChange
}
