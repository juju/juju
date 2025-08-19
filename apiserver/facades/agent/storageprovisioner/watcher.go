// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// stringSourcedWatcher is a generic watcher that listens to changes from a source
// watcher and maps the events to a specific type T using the provided mapper function.
// The mapper function should take the context and the events as input and return a slice of T or an error.
// The watcher will emit the mapped results on its Changes channel.
type stringSourcedWatcher[T any] struct {
	catacomb catacomb.Catacomb

	sourceWatcher watcher.StringsWatcher
	mapper        func(ctx context.Context, events ...string) ([]T, error)

	out chan []T
}

func newStringSourcedWatcher[T any](
	sourceWatcher watcher.StringsWatcher,
	mapper func(ctx context.Context, events ...string) ([]T, error),
) (*stringSourcedWatcher[T], error) {
	w := &stringSourcedWatcher[T]{
		sourceWatcher: sourceWatcher,
		mapper:        mapper,
		out:           make(chan []T, 1),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "attachment-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *stringSourcedWatcher[T]) loop() error {
	defer close(w.out)

	var (
		changes []T
		out     chan []T
		initial bool = true
	)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("source watcher %v closed", w.sourceWatcher)
			}

			if !initial && len(events) == 0 {
				continue
			}

			results, err := w.mapper(w.catacomb.Context(context.Background()), events...)
			if err != nil {
				return errors.Errorf("processing changes: %w", err)
			}

			if !initial && len(results) == 0 {
				continue
			}

			if changes == nil {
				changes = results
			} else {
				changes = append(changes, results...)
			}
			// If we have changes, we need to dispatch them.
			out = w.out
		case out <- changes:
			// We have dispatched. Reset changes for the next batch.
			changes = nil
			out = nil
			initial = false
		}
	}
}

// Changes returns the channel of the changes.
func (w *stringSourcedWatcher[T]) Changes() <-chan []T {
	return w.out
}

// Stop stops the watcher.
func (w *stringSourcedWatcher[T]) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher via its tomb.
func (w *stringSourcedWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *stringSourcedWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}

// machineStorageIdsWatcher defines the API wrapping a [corewatcher.MachineStorageIDsWatcher]
// watching machine/storage attachments. This watcher notifies about storage
// entities (volumes/filesystems) being attached to and detached from machines.
type machineStorageIdsWatcher struct {
	stop    func() error
	watcher watcher.MachineStorageIDsWatcher
}

func newMachineStorageIdsWatcherFromContext(
	_ context.Context,
	ctx facade.ModelContext,
) (facade.Facade, error) {
	return newMachineStorageIdsWatcher(
		ctx.WatcherRegistry(),
		ctx.Auth(),
		ctx.ID(),
		ctx.Dispose,
	)
}

func newMachineStorageIdsWatcher(
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	watcherID string,
	dispose func(),
) (*machineStorageIdsWatcher, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthModelAgent()) {
		return nil, apiservererrors.ErrPerm
	}

	w, err := watcherRegistry.Get(watcherID)
	if err != nil {
		return nil, errors.Errorf("getting watcher %q: %w", watcherID, err)
	}
	watcher, ok := w.(watcher.MachineStorageIDsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	stop := func() error {
		dispose()
		return watcherRegistry.Stop(watcherID)
	}
	return &machineStorageIdsWatcher{
		watcher: watcher,
		stop:    stop,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the machineStorageIdsWatcher.
func (w *machineStorageIdsWatcher) Next(ctx context.Context) (params.MachineStorageIdsWatchResult, error) {
	changes, err := internal.FirstResult(ctx, w.watcher)
	if err != nil {
		return params.MachineStorageIdsWatchResult{}, errors.Capture(err)
	}
	out := params.MachineStorageIdsWatchResult{
		Changes: make([]params.MachineStorageId, len(changes)),
	}
	for i, change := range changes {
		out.Changes[i] = params.MachineStorageId{
			MachineTag:    change.MachineTag,
			AttachmentTag: change.AttachmentTag,
		}
	}
	return out, nil
}

// Stop stops the watcher.
func (w *machineStorageIdsWatcher) Stop() error {
	return w.stop()
}
