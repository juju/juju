// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// API is an empty struct that backs the (not implemented) endpoint methods.
type API struct {
	watcherRegistry facade.WatcherRegistry
}

// AssignUnits is not implemented. It always returns a list of nil ErrorResults.
func (a *API) AssignUnits(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	return results, nil
}

// WatchUnitAssignments returns a strings watcher that is notified when new unit
// assignments are added to the db.
func (a *API) WatchUnitAssignments(ctx context.Context) (params.StringsWatchResult, error) {
	// Create a simple watcher that sends the empty string as initial event.
	w := newEmptyStringWatcher()

	watcherID, changes, err := internal.EnsureRegisterWatcher[[]string](ctx, a.watcherRegistry, w)
	if err != nil {
		return params.StringsWatchResult{}, errors.Capture(err)
	}
	return params.StringsWatchResult{
		StringsWatcherId: watcherID,
		Changes:          changes,
	}, nil
}

// newEmptyStringWatcher returns starts and returns a new empty string watcher,
// with an empty string as initial event.
func newEmptyStringWatcher() *emptyStringWatcher {
	changes := make(chan []string)

	w := &emptyStringWatcher{
		changes: changes,
	}
	w.tomb.Go(func() error {
		changes <- []string{""}
		defer close(changes)
		return w.loop()
	})

	return w
}

// emptyStringWatcher implements watcher.StringsWatcher.
type emptyStringWatcher struct {
	changes chan []string
	tomb    tomb.Tomb
}

// Changes returns the event channel for the empty string watcher.
func (w *emptyStringWatcher) Changes() <-chan []string {
	return w.changes
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *emptyStringWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *emptyStringWatcher) Wait() error {
	return w.tomb.Wait()
}

// Err returns any error encountered while the watcher
// has been running.
func (w *emptyStringWatcher) Err() error {
	return w.tomb.Err()
}

func (w *emptyStringWatcher) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// SetAgentStatus is not implemented. It always returns a list of nil
// ErrorResults.
func (a *API) SetAgentStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	return results, nil
}
