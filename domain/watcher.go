// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
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
	getDB  WatchableDBFactory
	logger logger.Logger
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
	tableName, summary string, changeMask changestream.ChangeType,
) (watcher.StringsWatcher, error) {
	w, err := f.NewNamespaceWatcher(
		eventsource.InitialNamespaceChanges("SELECT uuid from "+tableName),
		summary,
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
	summary string,
	filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNamespaceWatcher(base, initialQuery, summary, filterOption, filterOptions...)
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
	summary string,
	mapper eventsource.Mapper,
	filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNamespaceMapperWatcher(
		base, initialQuery, summary, mapper, filterOption, filterOptions...,
	)
}

// NewNotifyWatcher returns a new watcher that filters changes from the input
// base watcher's db/queue. A single filter option is required, though
// additional filter options can be provided.
func (f *WatcherFactory) NewNotifyWatcher(
	summary string,
	filter eventsource.FilterOption,
	filterOpts ...eventsource.FilterOption,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNotifyWatcher(base, summary, filter, filterOpts...)
}

// NewNotifyMapperWatcher returns a new watcher that receives changes from the
// input base watcher's db/queue. A single filter option is required, though
// additional filter options can be provided. Filtering of values is done first
// by the filter, and then subsequently by the mapper. Based on the mapper's
// logic a subset of them (or none) may be emitted.
func (f *WatcherFactory) NewNotifyMapperWatcher(
	summary string,
	mapper eventsource.Mapper,
	filter eventsource.FilterOption,
	filterOpts ...eventsource.FilterOption,
) (watcher.NotifyWatcher, error) {
	base, err := f.newBaseWatcher()
	if err != nil {
		return nil, errors.Errorf("creating base watcher: %w", err)
	}

	return eventsource.NewNotifyMapperWatcher(base, summary, mapper, filter, filterOpts...)
}

func (f *WatcherFactory) newBaseWatcher() (*eventsource.BaseWatcher, error) {
	watchableDB, err := f.getDB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	return eventsource.NewBaseWatcher(watchableDB, f.logger), nil
}
