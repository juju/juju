// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/names/v6"
)

// TODO returns a watcher for type T that sends an initial change
// with the empty value of type T.
func TODOStringsWatcher() StringsWatcher {
	var empty []string
	ch := make(chan []string, 1)
	ch <- empty
	w := &todoWatcher[[]string]{
		ch:   ch,
		done: make(chan struct{}),
	}
	return w
}

// TODO returns a watcher for type T that sends an initial change
// with the empty value of type T.
func TODONotifyWatcher() NotifyWatcher {
	var empty struct{}
	ch := make(chan struct{}, 1)
	ch <- empty
	w := &todoWatcher[struct{}]{
		ch:   ch,
		done: make(chan struct{}),
	}
	return w
}

type todoWatcher[T any] struct {
	ch   chan T
	done chan struct{}
}

func (w *todoWatcher[T]) Err() error {
	return nil
}

func (w *todoWatcher[T]) Stop() error {
	w.Kill()
	return nil
}

func (w *todoWatcher[T]) Kill() {
	select {
	case <-w.done:
	default:
		close(w.done)
		close(w.ch)
	}
}

func (w *todoWatcher[T]) Wait() error {
	<-w.done
	return nil
}

func (w *todoWatcher[T]) Changes() <-chan T {
	return w.ch
}

// Watcher is implemented by all watchers; the actual
// changes channel is returned by a watcher-specific
// Changes method.
type Watcher interface {
	// Kill asks the watcher to stop without waiting for it do so.
	Kill()
	// Wait waits for the watcher to die and returns any
	// error encountered when it was running.
	Wait() error
	// Stop kills the watcher, then waits for it to die.
	Stop() error
	// Err returns any error encountered while the watcher
	// has been running.
	Err() error
}

// NotifyWatcher generates signals when something changes, but it does not
// return any content for those changes
type NotifyWatcher interface {
	Watcher
	Changes() <-chan struct{}
}

// StringsWatcher generates signals when something changes, returning
// the changes as a list of strings.
type StringsWatcher interface {
	Watcher
	Changes() <-chan []string
}

// WatchModelLives returns a StringsWatcher that notifies of changes
// to any model life values. The watcher will not send any more events
// for a model after it has been observed to be Dead.
func (st *State) WatchModelLives() StringsWatcher {
	return TODOStringsWatcher()
}

// WatchStorageAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all storage instances attached to the
// specified unit.
func (sb *storageBackend) WatchStorageAttachments(unit names.UnitTag) StringsWatcher {
	return TODOStringsWatcher()
}

// WatchModelMachineStartTimes watches the non-container machines in the model
// for changes to the Life or AgentStartTime fields and reports them as a batch
// after the specified quiesceInterval time has passed without seeing any new
// change events.
func (st *State) WatchModelMachineStartTimes(quiesceInterval time.Duration) StringsWatcher {
	return TODOStringsWatcher()
}

// WatchModelMachines returns a StringsWatcher that notifies of changes to
// the lifecycles of the machines (but not containers) in the model.
func (st *State) WatchModelMachines() StringsWatcher {
	return TODOStringsWatcher()
}

// ErrStateClosed is returned from watchers if their underlying
// state connection has been closed.
var ErrStateClosed = fmt.Errorf("state has been closed")

// WatchControllerInfo returns a StringsWatcher for the controllers collection
func (st *State) WatchControllerInfo() StringsWatcher {
	return TODOStringsWatcher()
}

// Watch returns a watcher for observing changes to an application.
func (a *Application) Watch() NotifyWatcher {
	return TODONotifyWatcher()
}

// Watch returns a watcher for observing changes to a unit.
func (u *Unit) Watch() NotifyWatcher {
	return TODONotifyWatcher()
}

// Watch returns a watcher for observing changes to a model.
func (m *Model) Watch() NotifyWatcher {
	return TODONotifyWatcher()
}

// WatchModelEntityReferences returns a NotifyWatcher waiting for the Model
// Entity references to change for specified model.
func (st *State) WatchModelEntityReferences(mUUID string) NotifyWatcher {
	return TODONotifyWatcher()
}

// WatchStorageAttachment returns a watcher for observing changes
// to a storage attachment.
func (sb *storageBackend) WatchStorageAttachment(s names.StorageTag, u names.UnitTag) NotifyWatcher {
	return TODONotifyWatcher()
}

// WatchVolumeAttachment returns a watcher for observing changes
// to a volume attachment.
func (sb *storageBackend) WatchVolumeAttachment(host names.Tag, v names.VolumeTag) NotifyWatcher {
	return TODONotifyWatcher()
}

// WatchFilesystemAttachment returns a watcher for observing changes
// to a filesystem attachment.
func (sb *storageBackend) WatchFilesystemAttachment(host names.Tag, f names.FilesystemTag) NotifyWatcher {
	return TODONotifyWatcher()
}

// WatchCleanups starts and returns a CleanupWatcher.
func (st *State) WatchCleanups() NotifyWatcher {
	return TODONotifyWatcher()
}

// WatchActionLogs starts and returns a StringsWatcher that
// notifies on new log messages for a specified action being added.
// The strings are json encoded action messages.
func (st *State) WatchActionLogs(actionId string) StringsWatcher {
	return TODOStringsWatcher()
}
