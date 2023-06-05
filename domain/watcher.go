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

func (f *WatcherFactory) newBaseWatcher() (*eventsource.BaseWatcher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var err error

	if f.watchableDB == nil {
		if f.watchableDB, err = f.getDB(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	return eventsource.NewBaseWatcher(f.watchableDB, f.logger), nil
}

// NewUUIDsWatcher returns a new UUIDs watcher on the table and change mask
// provided.
func (f *WatcherFactory) NewUUIDsWatcher(
	changeMask changestream.ChangeType,
	tableName string,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Annotatef(err, "creating UUID watcher on namespace %s", tableName)
	}

	return eventsource.NewUUIDsWatcher(base, changeMask, tableName), nil
}

// NewKeysWatcher returns a new keys watcher on the table, key name and
// change mask provided.
func (f *WatcherFactory) NewKeysWatcher(
	changeMask changestream.ChangeType,
	tableName,
	keyName string,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Annotatef(err, "creating keys watcher on namespace %s and key %s", tableName, keyName)
	}

	return eventsource.NewKeysWatcher(base, changeMask, tableName, keyName), nil
}
