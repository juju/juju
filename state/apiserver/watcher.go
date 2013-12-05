// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/multiwatcher"
)

type srvClientAllWatcher struct {
	watcher   *multiwatcher.Watcher
	id        string
	resources *common.Resources
}

func (aw *srvClientAllWatcher) Next() (params.AllWatcherNextResults, error) {
	deltas, err := aw.watcher.Next()
	return params.AllWatcherNextResults{
		Deltas: deltas,
	}, err
}

func (w *srvClientAllWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

type srvNotifyWatcher struct {
	watcher   state.NotifyWatcher
	id        string
	resources *common.Resources
}

// Next returns when a change has occurred to the
// entity being watched since the most recent call to Next
// or the Watch call that created the NotifyWatcher.
func (w *srvNotifyWatcher) Next() error {
	if _, ok := <-w.watcher.Changes(); ok {
		return nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return err
}

// Stop stops the watcher.
func (w *srvNotifyWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// srvStringsWatcher notifies about changes for all entities of a
// given kind, sending the changes as a list of strings.
type srvStringsWatcher struct {
	watcher   state.StringsWatcher
	id        string
	resources *common.Resources
}

// Next returns when a change has occured to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvStringsWatcher.
func (w *srvStringsWatcher) Next() (params.StringsWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.StringsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.StringsWatchResult{}, err
}

// Stop stops the watcher.
func (w *srvStringsWatcher) Stop() error {
	return w.resources.Stop(w.id)
}

// srvRelationUnitsWatcher notifies about units entering and leaving
// the scope of a RelationUnit, and changes to the settings of those
// units known to have entered.
type srvRelationUnitsWatcher struct {
	watcher   state.RelationUnitsWatcher
	id        string
	resources *common.Resources
}

// Next returns when a change has occured to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationUnitsWatcher.
func (w *srvRelationUnitsWatcher) Next() (params.RelationUnitsWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.RelationUnitsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.RelationUnitsWatchResult{}, err
}

// Stop stops the watcher.
func (w *srvRelationUnitsWatcher) Stop() error {
	return w.resources.Stop(w.id)
}
