// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"sync"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
)

// WatchableDBFactory is a function that returns a WatchableDB or an error.
type WatchableDBFactory = func() (changestream.WatchableDB, error)

// WatcherFactory is a factory for creating watchers.
type WatcherFactory struct {
	mu sync.Mutex

	getDB       WatchableDBFactory
	watchableDB changestream.WatchableDB
	logger      logger.Logger
}

// NewWatcherFactory returns a new WatcherFactory.
func NewWatcherFactory(watchableDBFactory WatchableDBFactory, logger logger.Logger) *WatcherFactory {
	return &WatcherFactory{
		getDB:  watchableDBFactory,
		logger: logger,
	}
}

// NewUUIDsWatcher returns a watcher that emits the UUIDs for changes to the
// input table name that match the input mask.
func (f *WatcherFactory) NewUUIDsWatcher(
	tableName string, changeMask changestream.ChangeType,
) (watcher.StringsWatcher, error) {
	w, err := f.NewNamespaceWatcher(
		eventsource.InitialNamespaceChanges("SELECT uuid from "+tableName),
		eventsource.NamespaceFilter(tableName, changeMask),
	)
	return w, errors.Capture(err)
}

// NewNamespaceWatcher returns a new watcher that filters changes from the input
// base watcher's db/queue. Change-log events will be emitted only if the filter
// accepts them, and dispatching the notifications via the Changes channel. A
// filter option is required, though additional filter options can be provided.
func (f *WatcherFactory) NewNamespaceWatcher(
	initialQuery eventsource.NamespaceQuery,
	filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNamespaceWatcher(base, initialQuery, filterOption, filterOptions...)
}

// NewNamespaceMapperWatcher returns a new watcher that receives changes from
// the input base watcher's db/queue. Change-log events will be emitted only if
// the filter accepts them, and dispatching the notifications via the Changes
// channel, once the mapper has processed them. Filtering of values is done
// first by the filter, and then by the mapper. Based on the mapper's logic a
// subset of them (or none) may be emitted. A filter option is required, though
// additional filter options can be provided.
func (f *WatcherFactory) NewNamespaceMapperWatcher(
	initialQuery eventsource.NamespaceQuery,
	mapper eventsource.Mapper,
	filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNamespaceMapperWatcher(
		base, initialQuery, mapper, filterOption, filterOptions...,
	)
}

// NewNotifyWatcher returns a new watcher that filters changes from the input
// base watcher's db/queue. Change-log events will be emitted only if the filter
// accepts them, and dispatching the notifications via the Changes channel. A
// filter option is required, though additional filter options can be provided.
func (f *WatcherFactory) NewNotifyWatcher(
	filter eventsource.FilterOption,
	filterOpts ...eventsource.FilterOption,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNotifyWatcher(base, filter, filterOpts...)
}

// NewNotifyMapperWatcher returns a new watcher that receives changes from the
// input base watcher's db/queue. Change-log events will be emitted only if the
// filter accepts them, and dispatching the notifications via the Changes
// channel, once the mapper has processed them. Filtering of values is done
// first by the filter, and then by the mapper.
// A filter option is required, though additional filter options can be
// provided.
func (f *WatcherFactory) NewNotifyMapperWatcher(
	mapper eventsource.Mapper,
	filter eventsource.FilterOption,
	filterOpts ...eventsource.FilterOption,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNotifyMapperWatcher(base, mapper, filter, filterOpts...)
}

func (f *WatcherFactory) newBaseWatcher() (*eventsource.BaseWatcher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.watchableDB == nil {
		var err error
		if f.watchableDB, err = f.getDB(); err != nil {
			return nil, errors.Capture(err)
		}
	}

	return eventsource.NewBaseWatcher(f.watchableDB, f.logger), nil
}

// Everything below is deprecated and will be removed in the future.

// NewNamespaceNotifyWatcher returns a new namespace notify watcher
// for events based on the input change mask.
// Deprecated: use NewFilterWatcher instead.
func (f *WatcherFactory) NewNamespaceNotifyWatcher(
	namespace string, changeMask changestream.ChangeType,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNamespaceNotifyWatcher(base, namespace, changeMask), nil
}

// NewNamespaceNotifyMapperWatcher returns a new namespace notify watcher
// for events based on the input change mask and mapper.
// Deprecated: use NewFilterMapperWatcher instead.
func (f *WatcherFactory) NewNamespaceNotifyMapperWatcher(
	namespace string, changeMask changestream.ChangeType, mapper eventsource.Mapper,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNamespaceNotifyMapperWatcher(base, namespace, changeMask, mapper), nil
}

// NewValueWatcher returns a watcher for a particular change value
// in a namespace, based on the input change mask.
// Deprecated: use NewFilterWatcher instead.
func (f *WatcherFactory) NewValueWatcher(
	namespace, changeValue string, changeMask changestream.ChangeType,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewValueWatcher(base, namespace, changeValue, changeMask), nil
}

// NewValueMapperWatcher returns a watcher for a particular change value
// in a namespace, based on the input change mask and mapper.
// Deprecated: use NewFilterMapperWatcher instead.
func (f *WatcherFactory) NewValueMapperWatcher(
	namespace, changeValue string,
	changeMask changestream.ChangeType,
	mapper eventsource.Mapper,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewValueMapperWatcher(base, namespace, changeValue, changeMask, mapper), nil
}
