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

// srvLifecycleWatcher notifies about lifecycle changes for all
// entities of a given kind. See state.LifecycleWatcher.
type srvLifecycleWatcher struct {
	watcher   *state.LifecycleWatcher
	id        string
	resources *common.Resources
}

// Next returns when a change has occured to the lifecycle of an
// entity of the collection being watched since the most recent call
// to Next or the Watch call that created the srvLifecycleWatcher.
func (w *srvLifecycleWatcher) Next() (params.LifecycleWatchResults, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.LifecycleWatchResults{
			Ids: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = common.ErrStoppedWatcher
	}
	return params.LifecycleWatchResults{}, err
}

// Stop stops the watcher.
func (w *srvLifecycleWatcher) Stop() error {
	return w.resources.Stop(w.id)
}
