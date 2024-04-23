// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

type WatchableDBFactory = func() (changestream.WatchableDB, error)

type WatcherFactory struct {
	mu sync.Mutex

	getDB       WatchableDBFactory
	watchableDB changestream.WatchableDB
	logger      eventsource.Logger
}

func NewWatcherFactory(watchableDBFactory WatchableDBFactory, logger eventsource.Logger) *WatcherFactory {
	return &WatcherFactory{
		getDB:  watchableDBFactory,
		logger: logger,
	}
}

// NewUUIDsWatcher returns a watcher that emits the UUIDs for
// changes to the input table name that match the input mask.
func (f *WatcherFactory) NewUUIDsWatcher(
	tableName string, changeMask changestream.ChangeType,
) (watcher.StringsWatcher, error) {
	w, err := f.NewNamespaceWatcher(tableName, changeMask, eventsource.InitialNamespaceChanges("SELECT uuid from "+tableName))
	return w, errors.Trace(err)
}

// NewNamespaceWatcher returns a new namespace watcher
// for events based on the input change mask.
func (f *WatcherFactory) NewNamespaceWatcher(
	namespace string, changeMask changestream.ChangeType,
	initialStateQuery eventsource.NamespaceQuery,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Annotate(err, "creating base watcher")
	}

	return eventsource.NewNamespaceWatcher(base, namespace, changeMask, initialStateQuery), nil
}

// NewNamespaceMapperWatcher returns a new namespace watcher
// for events based on the input change mask and mapper.
func (f *WatcherFactory) NewNamespaceMapperWatcher(
	namespace string, changeMask changestream.ChangeType,
	initialStateQuery eventsource.NamespaceQuery,
	mapper eventsource.Mapper,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Annotate(err, "creating base watcher")
	}

	return eventsource.NewNamespaceMapperWatcher(
		base, namespace, changeMask,
		initialStateQuery, mapper,
	), nil
}

// NewValueWatcher returns a watcher for a particular change value
// in a namespace, based on the input change mask.
func (f *WatcherFactory) NewValueWatcher(
	namespace, changeValue string, changeMask changestream.ChangeType,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Annotate(err, "creating base watcher")
	}

	return eventsource.NewValueWatcher(base, namespace, changeValue, changeMask), nil
}

// NewValueMapperWatcher returns a watcher for a particular change value
// in a namespace, based on the input change mask and mapper.
func (f *WatcherFactory) NewValueMapperWatcher(
	namespace, changeValue string,
	changeMask changestream.ChangeType,
	mapper eventsource.Mapper,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Annotate(err, "creating base watcher")
	}

	return eventsource.NewValueMapperWatcher(base, namespace, changeValue, changeMask, mapper), nil
}

func (f *WatcherFactory) newBaseWatcher() (*eventsource.BaseWatcher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.watchableDB == nil {
		var err error
		if f.watchableDB, err = f.getDB(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	return eventsource.NewBaseWatcher(f.watchableDB, f.logger), nil
}
