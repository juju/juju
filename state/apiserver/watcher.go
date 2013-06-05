// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	apicommon "launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/multiwatcher"
)

type srvClientAllWatcher struct {
	*srvResource
}

func (aw srvClientAllWatcher) Next() (params.AllWatcherNextResults, error) {
	deltas, err := aw.resource.(*multiwatcher.Watcher).Next()
	return params.AllWatcherNextResults{
		Deltas: deltas,
	}, err
}

func (aw srvClientAllWatcher) Stop() error {
	return aw.resource.(*multiwatcher.Watcher).Stop()
}

type srvEntityWatcher struct {
	*srvResource
}

// Next returns when a change has occurred to the
// entity being watched since the most recent call to Next
// or the Watch call that created the EntityWatcher.
func (w srvEntityWatcher) Next() error {
	watcher := w.resource.(*state.EntityWatcher)
	if _, ok := <-watcher.Changes(); ok {
		return nil
	}
	err := watcher.Err()
	if err == nil {
		err = apicommon.ErrStoppedWatcher
	}
	return err
}

// srvLifecycleWatcher notifies about lifecycle changes for all
// entities of a given kind. See state.LifecycleWatcher.
type srvLifecycleWatcher struct {
	*srvResource
}

// Next returns when a change has occured to the lifecycle of an
// entity of the collection being watched since the most recent call
// to Next or the Watch call that created the srvLifecycleWatcher.
func (w srvLifecycleWatcher) Next() (params.LifecycleWatchResults, error) {
	watcher := w.resource.(*state.LifecycleWatcher)
	if changes, ok := <-watcher.Changes(); ok {
		return params.LifecycleWatchResults{
			Ids: changes,
		}, nil
	}
	err := watcher.Err()
	if err == nil {
		err = apicommon.ErrStoppedWatcher
	}
	return params.LifecycleWatchResults{}, err
}

// srvEnvironConfigWatcher notifies about changes to the environment
// configuration. See state.EnvironConfigWatcher.
type srvEnvironConfigWatcher struct {
	*srvResource
}

// Next returns when a change has occured to the environment
// configuration since the most recent call to Next or the Watch call
// that created the srvEnvironConfigWatcher.
func (w srvEnvironConfigWatcher) Next() (params.EnvironConfigWatchResults, error) {
	watcher := w.resource.(*state.EnvironConfigWatcher)
	if changes, ok := <-watcher.Changes(); ok {
		return params.EnvironConfigWatchResults{
			Config: changes.AllAttrs(),
		}, nil
	}
	err := watcher.Err()
	if err == nil {
		err = apicommon.ErrStoppedWatcher
	}
	return params.EnvironConfigWatchResults{}, err
}
